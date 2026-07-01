//go:build windows

package app

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
)

const (
	sysPollInterval = 2 * time.Second
	kbPerGB         = 1048576 // 1024 * 1024 kB per GB
)

// SystemStats holds CPU, memory, network, disk stats.
type SystemStats struct {
	CPUPercent    float64 `json:"cpuPercent"`
	MemPercent    float64 `json:"memPercent"`
	MemUsedGB     float64 `json:"memUsedGB"`
	MemTotalGB    float64 `json:"memTotalGB"`
	NetDownKBps   float64 `json:"netDownKBps"`   // Download speed in KB/s
	NetUpKBps     float64 `json:"netUpKBps"`     // Upload speed in KB/s
	LocalIP       string  `json:"localIP"`       // Primary local IP address
	DiskReadKBps  float64 `json:"diskReadKBps"`  // Disk read speed in KB/s
	DiskWriteKBps float64 `json:"diskWriteKBps"` // Disk write speed in KB/s
}

// SystemPoller periodically queries system stats using gopsutil.
// Note: gopsutil's cpu.Percent() handles CPU delta internally, so
// we don't need prevIdle/prevTotal fields like the Linux (/proc/stat) version.
type SystemPoller struct {
	emitter func(SystemStats)
	stopCh  chan struct{}

	// Network state
	prevRxBytes uint64
	prevTxBytes uint64
	netMu       sync.Mutex

	// Disk state
	prevDiskRead  uint64
	prevDiskWrite uint64
	diskMu        sync.Mutex
}

func NewSystemPoller(emitter func(SystemStats)) *SystemPoller {
	return &SystemPoller{
		emitter: emitter,
		stopCh:  make(chan struct{}),
	}
}

func (p *SystemPoller) Start() {
	// Prime CPU, network, and disk readings
	p.readCPU()
	p.readNetBytes()
	p.readDiskBytes()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("timo: sysinfo poller panic recovered: %v", r)
			}
		}()
		ticker := time.NewTicker(sysPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.poll()
			case <-p.stopCh:
				return
			}
		}
	}()
}

func (p *SystemPoller) Stop() {
	close(p.stopCh)
}

func (p *SystemPoller) poll() {
	cpu := p.readCPU()
	memTotal, memAvail := p.readMem()
	netDown, netUp := p.readNetSpeed()
	ip := getLocalIP()
	diskRead, diskWrite := p.readDiskSpeed()

	memUsed := memTotal - memAvail
	memPct := 0.0
	if memTotal > 0 {
		memPct = (memUsed / memTotal) * 100
	}

	p.emitter(SystemStats{
		CPUPercent:    cpu,
		MemPercent:    memPct,
		MemUsedGB:     memUsed,
		MemTotalGB:    memTotal,
		NetDownKBps:   netDown,
		NetUpKBps:     netUp,
		LocalIP:       ip,
		DiskReadKBps:  diskRead,
		DiskWriteKBps: diskWrite,
	})
}

// readCPU returns the CPU usage percentage using gopsutil.
func (p *SystemPoller) readCPU() float64 {
	// cpu.Percent with interval=0 returns the usage since last call
	percentages, err := cpu.Percent(0, false)
	if err != nil {
		log.Printf("sysinfo: failed to read CPU: %v", err)
		return 0
	}
	if len(percentages) == 0 {
		return 0
	}
	return percentages[0]
}

// readMem returns total and available memory in GB using gopsutil.
func (p *SystemPoller) readMem() (total, available float64) {
	v, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("sysinfo: failed to read memory: %v", err)
		return 0, 0
	}
	total = float64(v.Total) / 1024.0 / 1024.0 / 1024.0
	available = float64(v.Available) / 1024.0 / 1024.0 / 1024.0
	return total, available
}

// readNetBytes returns current cumulative Rx/Tx bytes using gopsutil.
func (p *SystemPoller) readNetBytes() (rx, tx uint64) {
	counters, err := psnet.IOCounters(false)
	if err != nil {
		log.Printf("sysinfo: failed to read network counters: %v", err)
		return 0, 0
	}
	for _, c := range counters {
		// Skip loopback interface
		if c.Name == "Loopback Pseudo-Interface 1" || c.Name == "lo" {
			continue
		}
		rx += c.BytesRecv
		tx += c.BytesSent
	}
	return rx, tx
}

// readNetSpeed calculates download/upload speed in KB/s since last poll.
func (p *SystemPoller) readNetSpeed() (downKBps, upKBps float64) {
	rx, tx := p.readNetBytes()

	p.netMu.Lock()
	dRx := rx - p.prevRxBytes
	dTx := tx - p.prevTxBytes
	p.prevRxBytes = rx
	p.prevTxBytes = tx
	p.netMu.Unlock()

	// Convert bytes per interval to KB/s
	intervalSec := float64(sysPollInterval) / float64(time.Second)
	downKBps = float64(dRx) / 1024.0 / intervalSec
	upKBps = float64(dTx) / 1024.0 / intervalSec
	return downKBps, upKBps
}

// readDiskBytes reads cumulative disk I/O bytes using gopsutil.
func (p *SystemPoller) readDiskBytes() (read, write uint64) {
	counters, err := disk.IOCounters()
	if err != nil {
		log.Printf("sysinfo: failed to read disk counters: %v", err)
		return 0, 0
	}
	for _, c := range counters {
		read += c.ReadBytes
		write += c.WriteBytes
	}
	return read, write
}

// readDiskSpeed calculates disk read/write speed in KB/s since last poll.
func (p *SystemPoller) readDiskSpeed() (readKBps, writeKBps float64) {
	read, write := p.readDiskBytes()

	p.diskMu.Lock()
	dRead := read - p.prevDiskRead
	dWrite := write - p.prevDiskWrite
	p.prevDiskRead = read
	p.prevDiskWrite = write
	p.diskMu.Unlock()

	intervalSec := float64(sysPollInterval) / float64(time.Second)
	readKBps = float64(dRead) / 1024.0 / intervalSec
	writeKBps = float64(dWrite) / 1024.0 / intervalSec
	return readKBps, writeKBps
}


// getLocalIP returns the primary non-loopback IPv4 address.
func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return ""
}

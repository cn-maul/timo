//go:build linux

package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sysPollInterval = 2 * time.Second
	kbPerGB         = 1048576 // 1024 * 1024 kB per GB
)

// SystemStats holds CPU, memory, and network stats.
type SystemStats struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemPercent   float64 `json:"memPercent"`
	MemUsedGB    float64 `json:"memUsedGB"`
	MemTotalGB   float64 `json:"memTotalGB"`
	NetDownKBps  float64 `json:"netDownKBps"`  // Download speed in KB/s
	NetUpKBps    float64 `json:"netUpKBps"`    // Upload speed in KB/s
	LocalIP      string  `json:"localIP"`      // Primary local IP address
}

// SystemPoller periodically reads /proc for CPU, memory, and network stats.
type SystemPoller struct {
	emitter func(SystemStats)
	stopCh  chan struct{}

	// CPU state
	prevIdle  uint64
	prevTotal uint64
	cpuMu     sync.Mutex

	// Network state
	prevRxBytes uint64
	prevTxBytes uint64
	netMu       sync.Mutex
}

func NewSystemPoller(emitter func(SystemStats)) *SystemPoller {
	return &SystemPoller{
		emitter: emitter,
		stopCh:  make(chan struct{}),
	}
}

func (p *SystemPoller) Start() {
	// Prime CPU and network readings
	p.readCPU()
	p.readNetBytes()
	go func() {
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

	memUsed := memTotal - memAvail
	memPct := 0.0
	if memTotal > 0 {
		memPct = (memUsed / memTotal) * 100
	}

	p.emitter(SystemStats{
		CPUPercent:  cpu,
		MemPercent:  memPct,
		MemUsedGB:   memUsed,
		MemTotalGB:  memTotal,
		NetDownKBps: netDown,
		NetUpKBps:   netUp,
		LocalIP:     ip,
	})
}

func (p *SystemPoller) readCPU() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		log.Printf("sysinfo: failed to open /proc/stat: %v", err)
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0
		}

		var vals [4]uint64
		for i := 0; i < 4; i++ {
			v, err := strconv.ParseUint(fields[i+1], 10, 64)
			if err != nil {
				log.Printf("sysinfo: failed to parse cpu field %q: %v", fields[i+1], err)
			}
			vals[i] = v
		}
		idle := vals[3]
		var total uint64
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				log.Printf("sysinfo: failed to parse cpu total field %q: %v", fields[i], err)
			}
			total += v
		}

		p.cpuMu.Lock()
		dIdle := idle - p.prevIdle
		dTotal := total - p.prevTotal
		p.prevIdle = idle
		p.prevTotal = total
		p.cpuMu.Unlock()

		if dTotal == 0 {
			return 0
		}
		return (1.0 - float64(dIdle)/float64(dTotal)) * 100
	}
	return 0
}

func (p *SystemPoller) readMem() (total, available float64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		log.Printf("sysinfo: failed to open /proc/meminfo: %v", err)
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		valGB := val / kbPerGB
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = valGB
		case strings.HasPrefix(line, "MemAvailable:"):
			available = valGB
		}
		if total > 0 && available > 0 {
			break
		}
	}
	return
}

// readNetBytes returns current cumulative Rx/Tx bytes from /proc/net/dev.
func (p *SystemPoller) readNetBytes() (rx, tx uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		log.Printf("sysinfo: failed to open /proc/net/dev: %v", err)
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip header lines
		if strings.Contains(line, "|") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// Interface name may have trailing colon: "eth0:"
		iface := strings.TrimSuffix(fields[0], ":")
		// Skip loopback and virtual interfaces
		if iface == "lo" || strings.HasPrefix(iface, "veth") || strings.HasPrefix(iface, "docker") {
			continue
		}
		// fields[1] = receive bytes, fields[9] = transmit bytes
		rxVal, err1 := strconv.ParseUint(fields[1], 10, 64)
		txVal, err2 := strconv.ParseUint(fields[9], 10, 64)
		if err1 == nil && err2 == nil {
			rx += rxVal
			tx += txVal
		}
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
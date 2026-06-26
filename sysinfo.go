//go:build linux

package main

import (
	"bufio"
	"log"
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

// SystemStats holds CPU and memory usage.
type SystemStats struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemPercent float64 `json:"memPercent"`
	MemUsedGB  float64 `json:"memUsedGB"`
	MemTotalGB float64 `json:"memTotalGB"`
}

// SystemPoller periodically reads /proc for CPU and memory stats.
type SystemPoller struct {
	emitter func(SystemStats)
	stopCh  chan struct{}

	// CPU state
	prevIdle  uint64
	prevTotal uint64
	cpuMu     sync.Mutex
}

func NewSystemPoller(emitter func(SystemStats)) *SystemPoller {
	return &SystemPoller{
		emitter: emitter,
		stopCh:  make(chan struct{}),
	}
}

func (p *SystemPoller) Start() {
	// Prime CPU readings
	p.readCPU()
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

	memUsed := memTotal - memAvail
	memPct := 0.0
	if memTotal > 0 {
		memPct = (memUsed / memTotal) * 100
	}

	p.emitter(SystemStats{
		CPUPercent: cpu,
		MemPercent: memPct,
		MemUsedGB:  memUsed,
		MemTotalGB: memTotal,
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
		// user + nice + system + idle (+ iowait + irq + softirq + steal)
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

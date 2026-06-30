//go:build linux

package app

import (
	"os"
	"strings"
	"log"
	"time"
)

var selfPID string

func init() {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		log.Printf("timo: failed to read /proc/self/stat: %v", err)
		return
	}
	fields := strings.SplitN(string(data), " ", 2)
	if len(fields) > 0 {
		selfPID = fields[0]
	}
}

// ProcessMonitor watches for specified processes and sends status updates.
// Tracks process counts per name; only emits "done" when ALL instances exit.
type ProcessMonitor struct {
	emitter    func(Notification)
	stopCh     chan struct{}
	lastCount  map[string]int  // how many instances of each process were running
	watchNames []string
}

func NewProcessMonitor(watchNames []string, emitter func(Notification)) *ProcessMonitor {
	pm := &ProcessMonitor{
		emitter:    emitter,
		stopCh:     make(chan struct{}),
		lastCount:  make(map[string]int),
		watchNames: watchNames,
	}
	// 初始化时立即记录当前进程计数，避免首次 check 前的遗漏窗口
	for _, name := range watchNames {
		pm.lastCount[name] = countProcesses(name)
	}
	return pm
}

func (m *ProcessMonitor) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("timo: process monitor panic recovered: %v", r)
			}
		}()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.check()
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *ProcessMonitor) Stop() {
	close(m.stopCh)
}

func (m *ProcessMonitor) check() {
	for _, name := range m.watchNames {
		count := countProcesses(name)
		prev := m.lastCount[name]
		// Only emit done when transitioning from "some running" to "none running"
		if prev > 0 && count == 0 {
			m.emitter(Notification{Type: name + "-done", Message: msgTaskComplete})
		}
		m.lastCount[name] = count
	}
}

// isProcessRunning returns true if any process with the given name is running.
func isProcessRunning(name string) bool {
	return countProcesses(name) > 0
}

// countProcesses returns the number of running processes with the given name.
func countProcesses(name string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if len(dirName) == 0 || dirName[0] < '0' || dirName[0] > '9' {
			continue
		}
		// Skip our own PID – we want to count external processes only.
		if dirName == selfPID {
			continue
		}
		comm, err := os.ReadFile("/proc/" + dirName + "/comm")
		if err != nil {
			continue
		}
		procName := strings.TrimSpace(string(comm))
		if procName == name {
			count++
		}
	}
	return count
}

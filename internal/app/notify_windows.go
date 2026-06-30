//go:build windows

package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/windows"
)

const pipePath = `\\.\pipe\timo`

// NotifyServer listens on a Windows named pipe for notifications.
type NotifyServer struct {
	listener net.Listener
	callback func(Notification)
	stopCh   chan struct{}
}

func NewNotifyServer(callback func(Notification)) *NotifyServer {
	return &NotifyServer{
		callback: callback,
		stopCh:   make(chan struct{}),
	}
}

func (s *NotifyServer) Start() error {
	// Create a named pipe listener
	ln, err := listenPipe(pipePath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", pipePath, err)
	}
	s.listener = ln

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-s.stopCh:
					return
				default:
					continue
				}
			}
			go s.handleConn(conn)
		}
	}()

	return nil
}

func (s *NotifyServer) handleConn(conn net.Conn) {
	defer conn.Close()
	var notif Notification
	if err := json.NewDecoder(conn).Decode(&notif); err != nil {
		return
	}
	if s.callback != nil {
		s.callback(notif)
	}
}

func (s *NotifyServer) Stop() {
	close(s.stopCh)
	if s.listener != nil {
		s.listener.Close()
	}
}

// listenPipe creates a named pipe listener on Windows.
func listenPipe(path string) (net.Listener, error) {
	return &pipeListener{
		path:   path,
		stopCh: make(chan struct{}),
	}, nil
}

// pipeListener implements net.Listener for Windows named pipes.
type pipeListener struct {
	path   string
	stopCh chan struct{}
	mu     bool
}

func (l *pipeListener) Accept() (net.Conn, error) {
	// Create a new pipe instance for each connection
	pathPtr, err := windows.UTF16PtrFromString(l.path)
	if err != nil {
		return nil, err
	}

	// CreateNamedPipe with default security (NULL means default security descriptor)
	handle, err := windows.CreateNamedPipe(
		pathPtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES,
		4096, // output buffer size
		4096, // input buffer size
		0,    // default timeout (50ms default)
		nil,  // default security attributes
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create named pipe: %w", err)
	}

	// Wait for a client to connect
	err = windows.ConnectNamedPipe(handle, nil)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}

	return &pipeConn{handle: handle}, nil
}

func (l *pipeListener) Close() error {
	return nil
}

func (l *pipeListener) Addr() net.Addr {
	return &pipeAddr{path: l.path}
}

// pipeAddr implements net.Addr for named pipes.
type pipeAddr struct {
	path string
}

func (a *pipeAddr) Network() string { return "pipe" }
func (a *pipeAddr) String() string  { return a.path }

// pipeConn implements net.Conn for Windows named pipes.
type pipeConn struct {
	handle windows.Handle
}

func (c *pipeConn) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(c.handle, b, &n, nil)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

func (c *pipeConn) Write(b []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(c.handle, b, &n, nil)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (c *pipeConn) Close() error {
	return windows.CloseHandle(c.handle)
}

func (c *pipeConn) LocalAddr() net.Addr {
	return &pipeAddr{path: "local"}
}

func (c *pipeConn) RemoteAddr() net.Addr {
	return &pipeAddr{path: "remote"}
}

func (c *pipeConn) SetDeadline(t time.Time) error {
	// Not supported for named pipes
	return nil
}

func (c *pipeConn) SetReadDeadline(t time.Time) error {
	// Not supported for named pipes
	return nil
}

func (c *pipeConn) SetWriteDeadline(t time.Time) error {
	// Not supported for named pipes
	return nil
}

// ── Process monitor for Claude Code (Windows version) ──

// ProcessMonitor watches for specified processes and sends status updates.
// Tracks process counts per name; only emits "done" when ALL instances exit.
type ProcessMonitor struct {
	emitter    func(Notification)
	stopCh     chan struct{}
	lastCount  map[string]int
	watchNames []string
}

func NewProcessMonitor(watchNames []string, emitter func(Notification)) *ProcessMonitor {
	pm := &ProcessMonitor{
		emitter:    emitter,
		stopCh:     make(chan struct{}),
		lastCount:  make(map[string]int),
		watchNames: watchNames,
	}
	// Initialize with current process counts
	for _, name := range watchNames {
		pm.lastCount[name] = countProcesses(name)
	}
	return pm
}

func (m *ProcessMonitor) Start() {
	go func() {
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

// countProcesses returns the number of running processes with the given name on Windows.
// Uses gopsutil to enumerate processes.
func countProcesses(name string) int {
	procs, err := process.Processes()
	if err != nil {
		return 0
	}

	count := 0
	for _, proc := range procs {
		executable, err := proc.Name()
		if err != nil {
			continue
		}

		// Match process name (case-insensitive on Windows)
		if !strings.EqualFold(executable, name) {
			continue
		}

		// Exclude timo itself
		cmdline, err := proc.Cmdline()
		if err != nil {
			count++
			continue
		}

		if !strings.Contains(cmdline, "timo") {
			count++
		}
	}

	return count
}

// ── CLI ──

func SendNotification(notif Notification) error {
	conn, err := dialPipe(pipePath)
	if err != nil {
		return fmt.Errorf("Timo is not running. Start Timo GUI first, then retry. %w", err)
	}
	defer conn.Close()
	return json.NewEncoder(conn).Encode(notif)
}

// dialPipe connects to a Windows named pipe.
func dialPipe(path string) (net.Conn, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	// Wait for the pipe to be available (with timeout)
	start := time.Now()
	timeout := 5 * time.Second

	for {
		handle, err := windows.CreateFile(
			pathPtr,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			0, // no overlapped
			0,
		)
		if err == nil {
			return &pipeConn{handle: handle}, nil
		}

		if time.Since(start) > timeout {
			return nil, fmt.Errorf("timeout waiting for pipe: %w", err)
		}

		// If pipe is busy, wait and retry
		if err == windows.ERROR_PIPE_BUSY {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		return nil, err
	}
}

func RunCLI() {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	var notifType, notifMsg, notifTool, notifDir, notifTopic string
	fs.StringVar(&notifType, "type", "", "Notification type")
	fs.StringVar(&notifMsg, "msg", "", "Human-readable message")
	fs.StringVar(&notifTool, "tool", "", "Current tool name")
	fs.StringVar(&notifDir, "dir", "", "Working directory")
	fs.StringVar(&notifTopic, "topic", "", "User prompt text")

	args := os.Args[2:]
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Usage: %s notify --type <type> [--msg <message>] [--tool <tool>] [--dir <workdir>] [--topic <topic>]\n", os.Args[0])
		os.Exit(1)
	}

	notif := Notification{
		Type:    notifType,
		Message: notifMsg,
		Tool:    notifTool,
		WorkDir: notifDir,
		Topic:   notifTopic,
	}

	// Read stdin for hook data from Claude Code / Reasonix
	readStdinHookData(&notif)

	if err := SendNotification(notif); err != nil {
		// Best-effort: if Timo isn't running, the notification is silently dropped.
		// This avoids non-zero exit codes triggering warnings in Reasonix/Claude Code hooks.
		// Debug hint: start Timo first to receive notifications.
		os.Exit(0)
	}
}

func IsCLI() bool {
	return len(os.Args) > 1 && (os.Args[1] == "notify" || os.Args[1] == "setup" || os.Args[1] == "setup-reasonix")
}

func GetSocketPath() string {
	return pipePath
}

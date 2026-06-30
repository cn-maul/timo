//go:build linux

package app

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// We use a fixed socket path since tryClaimInstance() enforces single-instance.
// This avoids the fragile PID-file → PID → socket-path dance that can break
// when /tmp is cleaned, PIDs are recycled, or files race.
const (
	pidFilePath  = "/tmp/timo.pid"
	socketPath   = "/tmp/timo.sock"
)

// getSocketPath returns the fixed Unix socket path for this instance.
func getSocketPath() string {
	return socketPath
}

// ReadSocketPathFromPID reads the PID file and returns the socket path.
// If the PID file is missing or the process is dead, returns empty string.
func ReadSocketPathFromPID() string {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return ""
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return ""
	}
	// Verify the process is still alive
	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		os.Remove(pidFilePath)
		return ""
	}
	// On Unix, FindProcess always succeeds; check by sending signal 0
	if err := proc.Signal(os.Signal(nil)); err != nil {
		// Process is dead
		os.Remove(pidFilePath)
		return ""
	}
	return socketPath
}

// tryClaimInstance attempts to claim the Timo instance slot.
// Returns the socket path and nil if this is the only instance.
// Returns an error if another instance is already running.
func tryClaimInstance() (string, error) {
	if existingPath := ReadSocketPathFromPID(); existingPath != "" {
		return "", fmt.Errorf("another Timo instance is already running")
	}
	// Write our PID file
	if err := os.WriteFile(pidFilePath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		return "", fmt.Errorf("cannot write PID file %s: %w", pidFilePath, err)
	}
	return socketPath, nil
}

// releaseInstance removes the PID file if it belongs to us.
func releaseInstance() {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return
	}
	if strings.TrimSpace(string(data)) == strconv.Itoa(os.Getpid()) {
		os.Remove(pidFilePath)
	}
}

// NotifyServer listens on a Unix domain socket for notifications.
type NotifyServer struct {
	listener net.Listener
	callback func(Notification)
	stopCh   chan struct{}
	sockPath string
}

func NewNotifyServer(callback func(Notification)) *NotifyServer {
	return &NotifyServer{
		callback: callback,
		stopCh:   make(chan struct{}),
	}
}

func (s *NotifyServer) Start() error {
	sockPath, err := tryClaimInstance()
	if err != nil {
		return err
	}
	s.sockPath = sockPath

	// Ensure old socket is removed (shouldn't exist, but be safe)
	os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		releaseInstance()
		return fmt.Errorf("failed to listen on %s: %w", sockPath, err)
	}
	// Set permissions immediately after socket creation to minimize exposure
	os.Chmod(sockPath, 0600)
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
	// Set a read deadline to prevent hanging on partial/malicious input
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
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
	if s.sockPath != "" {
		os.Remove(s.sockPath)
	}
	releaseInstance()
}

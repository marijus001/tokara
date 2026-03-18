package daemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PidFile returns the path to the PID file.
func PidFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tokara", "proxy.pid")
}

// LogFile returns the path to the daemon log file.
func LogFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tokara", "proxy.log")
}

// IsRunning checks if a daemon is currently running.
// Returns the PID if running, 0 if not.
func IsRunning() int {
	data, err := os.ReadFile(PidFile())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	// Check if process exists
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process doesn't exist, clean up stale PID file
		os.Remove(PidFile())
		return 0
	}
	return pid
}

// IsPortInUse checks if a port is already bound.
func IsPortInUse(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Start launches the proxy as a background daemon.
// It re-executes the current binary with the --daemon-child flag.
func Start(binaryPath string, port int) (int, error) {
	if pid := IsRunning(); pid != 0 {
		return pid, fmt.Errorf("daemon already running (pid %d)", pid)
	}
	if IsPortInUse(port) {
		return 0, fmt.Errorf("port %d already in use — another process may be running. Use `lsof -i :%d` to check", port, port)
	}

	// Ensure ~/.tokara directory exists
	home, _ := os.UserHomeDir()
	os.MkdirAll(filepath.Join(home, ".tokara"), 0755)

	logFile, err := os.OpenFile(LogFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("cannot open log file: %w", err)
	}

	cmd := exec.Command(binaryPath, "--daemon-child", "--port", strconv.Itoa(port))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("failed to start daemon: %w", err)
	}

	pid := cmd.Process.Pid
	if err := WritePid(pid); err != nil {
		return pid, fmt.Errorf("daemon started (pid %d) but failed to write PID file: %w", pid, err)
	}

	// Detach — don't wait for the child
	cmd.Process.Release()
	logFile.Close()

	// Verify the daemon actually started by checking the port
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if IsPortInUse(port) {
			return pid, nil
		}
	}

	// Check if process is still alive
	if proc, err := os.FindProcess(pid); err == nil {
		if proc.Signal(syscall.Signal(0)) != nil {
			os.Remove(PidFile())
			return 0, fmt.Errorf("daemon exited immediately — check %s for errors", LogFile())
		}
	}

	return pid, nil
}

// Stop kills the running daemon.
func Stop() error {
	pid := IsRunning()
	if pid == 0 {
		return fmt.Errorf("no daemon running")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("cannot find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("cannot stop daemon (pid %d): %w", pid, err)
	}

	os.Remove(PidFile())
	return nil
}

// WritePid writes the PID to the PID file.
func WritePid(pid int) error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tokara")
	os.MkdirAll(dir, 0755)
	return os.WriteFile(PidFile(), []byte(strconv.Itoa(pid)), 0644)
}

// RemovePid removes the PID file.
func RemovePid() {
	os.Remove(PidFile())
}

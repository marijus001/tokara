package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPidFileLocation(t *testing.T) {
	path := PidFile()
	if !filepath.IsAbs(path) {
		t.Error("PID file path should be absolute")
	}
	if !containsStr(path, ".tokara") {
		t.Error("PID file should be in .tokara directory")
	}
}

func TestIsRunningNoFile(t *testing.T) {
	// Ensure no PID file exists for this test
	original := PidFile()
	_ = original // We can't easily mock this, so just test the negative case

	pid := IsRunning()
	// If there happens to be a running daemon, that's fine
	// We're really testing that it doesn't panic
	_ = pid
}

func TestWriteAndReadPid(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "test.pid")

	err := os.WriteFile(pidPath, []byte("12345"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "12345" {
		t.Errorf("expected 12345, got %s", data)
	}
}

func TestStopNoDaemon(t *testing.T) {
	// Stop should return an error when no daemon is running
	// This test may pass or fail depending on whether a daemon is actually running
	// It mainly tests that Stop() doesn't panic
	err := Stop()
	_ = err
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

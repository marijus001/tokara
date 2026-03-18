package detect

import (
	"testing"
)

func TestAllToolsCount(t *testing.T) {
	tools := AllTools("http://localhost:18741")
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}
}

func TestAllToolsHaveIDs(t *testing.T) {
	tools := AllTools("http://localhost:18741")
	for _, tool := range tools {
		if tool.ID == "" {
			t.Errorf("tool %s has empty ID", tool.Name)
		}
		if tool.Name == "" {
			t.Errorf("tool with ID %s has empty name", tool.ID)
		}
	}
}

func TestDetectDoesNotPanic(t *testing.T) {
	tools := AllTools("http://localhost:18741")
	// Just verify detection doesn't panic on any tool
	for _, tool := range tools {
		_ = Detect(tool)
	}
}

func TestShellProfileNotEmpty(t *testing.T) {
	profile := ShellProfile()
	if profile == "" {
		t.Error("shell profile path should not be empty")
	}
}

func TestDetectSelectedSubset(t *testing.T) {
	tools := AllTools("http://localhost:18741")
	// Select indices 0 and 2
	selected := DetectSelected(tools, []int{0, 2})
	// Should return 0-2 tools (depending on what's installed)
	if len(selected) > 2 {
		t.Errorf("should return at most 2 tools, got %d", len(selected))
	}
}

func TestDetectSelectedOutOfBounds(t *testing.T) {
	tools := AllTools("http://localhost:18741")
	// Invalid indices should not panic
	selected := DetectSelected(tools, []int{-1, 100})
	if len(selected) != 0 {
		t.Errorf("expected 0 results for invalid indices, got %d", len(selected))
	}
}

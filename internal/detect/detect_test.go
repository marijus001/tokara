package detect

import (
	"testing"
)

func TestAllToolsCount(t *testing.T) {
	tools := AllTools("http://localhost:18741")
	if len(tools) != 6 {
		t.Errorf("expected 6 tools, got %d", len(tools))
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
	for _, tool := range tools {
		_ = Detect(tool)
	}
}

func TestCanLaunch(t *testing.T) {
	tools := AllTools("http://localhost:18741")
	for _, tool := range tools {
		switch tool.ID {
		case "claude", "codex", "aider", "continue":
			if !CanLaunch(tool) {
				t.Errorf("%s should be launchable", tool.ID)
			}
		case "cursor", "windsurf":
			if CanLaunch(tool) {
				t.Errorf("%s should not be launchable (GUI tool)", tool.ID)
			}
		}
	}
}

func TestSDKEnvVars(t *testing.T) {
	vars := SDKEnvVars("http://localhost:18741")
	if len(vars) != 4 {
		t.Errorf("expected 4 SDK env vars, got %d", len(vars))
	}
	if vars["ANTHROPIC_BASE_URL"] != "http://localhost:18741" {
		t.Errorf("ANTHROPIC_BASE_URL wrong: %s", vars["ANTHROPIC_BASE_URL"])
	}
}

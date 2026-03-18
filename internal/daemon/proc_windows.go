//go:build windows

package daemon

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// Windows doesn't support Setsid; the process detaches naturally
	// when started without a console via CREATE_NEW_PROCESS_GROUP
}

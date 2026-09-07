//go:build windows

package tool

import "os/exec"

func setpgid(*exec.Cmd) {}

func terminate(cmd *exec.Cmd, _ bool) error {
	return cmd.Process.Kill()
}

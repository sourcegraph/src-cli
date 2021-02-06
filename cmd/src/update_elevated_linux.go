// +build !windows

package main

import (
	"os"
	"os/exec"
)

func runWithElevatedPrivilege() error {
	cmd := exec.Command("sudo", "/bin/sh", "-c", "./src update")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

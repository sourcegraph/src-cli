// +build windows

package main

import (
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// References:
// - https://gist.github.com/jerblack/d0eb182cc5a1c1d92d92a4c4fcc416c6
// - https://www.codeproject.com/Articles/320748/Elevating-During-Runtime
func runWithElevatedPrivilege() error {
	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	var showCmd int32 = 1 //SW_NORMAL

	err := windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		return err
	}

	return nil
}

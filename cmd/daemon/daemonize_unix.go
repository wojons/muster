//go:build !windows

package main

import "syscall"

// daemonProcAttr returns the process attributes used when re-execing the
// daemon as a detached background child.
//
// On Unix the child must call setsid(2): it becomes the leader of a new
// session with no controlling terminal, so it survives the parent shell
// (and its process group) going away. The tag mirrors pidfile_unix.go and
// covers every non-Windows target the project builds for (linux, darwin).
func daemonProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

//go:build windows

package main

import "syscall"

// Windows process-creation flags used to detach the daemon child from the
// parent's console. The standard syscall package does not export
// DETACHED_PROCESS, so it is declared here (CreateProcess flags, see
// winbase.h); CREATE_NEW_PROCESS_GROUP is re-exported locally so both
// flags read together.
const (
	// createNewProcessGroup is CREATE_NEW_PROCESS_GROUP (0x00000200): the
	// child gets its own console process group, so Ctrl+C in the parent's
	// console is not delivered to it.
	createNewProcessGroup uint32 = 0x00000200
	// detachedProcess is DETACHED_PROCESS (0x00000008): the child is not
	// attached to the parent's console, the Windows equivalent of the Unix
	// setsid(2) session detach used by daemonProcAttr on Unix.
	detachedProcess uint32 = 0x00000008
)

// daemonProcAttr returns the process attributes used when re-execing the
// daemon as a detached background child.
//
// Windows has no setsid(2); the equivalent detach is starting the child in
// its own process group without an inherited console.
func daemonProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}

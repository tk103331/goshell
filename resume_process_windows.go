//go:build windows

package main

import (
	"syscall"
)

// lookupProcess tells about a process, as Windows says.
func lookupProcess(pid int) (processStatus, bool) {
	st := processStatus{uid: -1}
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// the process is gone
		return st, true
	}
	syscall.CloseHandle(handle)
	st.alive = true
	return st, true
}

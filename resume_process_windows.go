//go:build windows

package main

import (
	"syscall"
)

// processQueryLimitedInformation is the right Windows asks for when it only
// wants to know about a process. The standard syscall package of Go does
// not name it.
const processQueryLimitedInformation = 0x1000

// lookupProcess tells about a process, as Windows says.
func lookupProcess(pid int) (processStatus, bool) {
	st := processStatus{uid: -1}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		// the process is gone
		return st, true
	}
	syscall.CloseHandle(handle)
	st.alive = true
	return st, true
}

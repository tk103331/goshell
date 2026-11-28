//go:build !linux && !darwin && !windows

package main

// lookupProcess cannot tell about a process here, so the app takes the
// reports for what they say.
func lookupProcess(pid int) (processStatus, bool) {
	return processStatus{uid: -1}, false
}

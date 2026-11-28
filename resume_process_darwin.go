//go:build darwin

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// lookupProcess tells about a process, as ps says.
func lookupProcess(pid int) (processStatus, bool) {
	st := processStatus{uid: -1}
	out, err := exec.Command("ps", "-o", "lstart=,uid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		// ps says nothing of a process that is gone
		return st, true
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return st, true
	}
	// The line is "Sat Nov 28 16:53:54 2026       501": the time the
	// process started, and the user it runs as.
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return st, false
	}
	uid, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return st, false
	}
	st.alive = true
	st.uid = uid
	st.start = strings.Join(fields[:len(fields)-1], " ")
	return st, true
}

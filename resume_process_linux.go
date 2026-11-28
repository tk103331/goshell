//go:build linux

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// lookupProcess tells about a process, as the /proc of Linux says.
func lookupProcess(pid int) (processStatus, bool) {
	st := processStatus{uid: -1}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		if os.IsNotExist(err) {
			// the process is gone
			return st, true
		}
		return st, false
	}
	// The line is "pid (comm) state ...", where comm may hold spaces and
	// parentheses: the fields after it are numbered from the state.
	line := string(data)
	i := strings.LastIndex(line, ")")
	if i < 0 {
		return st, false
	}
	fields := strings.Fields(line[i+1:])
	// The start time is the 22nd field of the line, which is the 20th
	// after the state.
	if len(fields) < 20 {
		return st, false
	}
	st.alive = true
	st.start = fields[19]
	st.uid = processUID(pid)
	return st, true
}

// processUID reads the user a process runs as.
func processUID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		value, ok := strings.CutPrefix(line, "Uid:")
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return -1
		}
		uid, err := strconv.Atoi(fields[0])
		if err != nil {
			return -1
		}
		return uid
	}
	return -1
}

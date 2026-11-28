package main

// The protocol asks the terminal to check the program a report is about: it
// should be running, and it should run as the user the app runs as. The
// system tells on Linux, macOS and Windows; elsewhere the app takes the
// report for what it says.

// processStatus is what the system says about a process.
type processStatus struct {
	alive bool
	uid   int    // the user it runs as, -1 when unknown
	start string // when it started, "" when unknown
}

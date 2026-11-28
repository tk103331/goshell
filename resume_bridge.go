package main

import (
	"sync"
)

// The library keeps the OSC sequences it does not implement for the
// embedder, and mygo's terminal does not expose that callback yet: the
// bridge in program_status_bridge.go reaches it, and the Program Resume
// Protocol rides on it, as the Program Status Protocol does.

// resumeState holds what the app knows of the terminals that received
// reports: the token of each, and the store its records go to.
var resumeState struct {
	mu    sync.RWMutex
	terms map[uintptr]*resumeTerminal
}

// attachResume gives a terminal its token, so that the reports of its
// programs can be checked, and asks the library for the sequences it does
// not know, which the reports come through. A terminal without a token gets
// no reports: a remote session has no token, as the token must not leave
// the machine.
func attachResume(tab *Tab, token string, store *resumeStore) {
	if tab == nil || tab.term == nil || token == "" || store == nil {
		return
	}
	handle := terminalHandle(tab.term)
	if handle == 0 {
		return
	}
	if !enableUnknownSequences(handle) {
		return
	}
	state := &resumeTerminal{
		token: token,
		store: store,
		pids:  map[int]bool{},
		last:  map[int]int64{},
	}
	resumeState.mu.Lock()
	if resumeState.terms == nil {
		resumeState.terms = map[uintptr]*resumeTerminal{}
	}
	resumeState.terms[handle] = state
	resumeState.mu.Unlock()
	tab.resume = state
}

// detachResume forgets a terminal being closed, and drops the records of
// the programs that reported to it when the user closed the terminal on
// purpose: the protocol says a terminal drops them then, and keeps them
// when it ends for any other reason.
func detachResume(tab *Tab, discard bool) {
	if tab == nil || tab.term == nil || tab.resume == nil {
		return
	}
	handle := terminalHandle(tab.term)
	resumeState.mu.Lock()
	// the handle may be another terminal's by now, as the library gives
	// them out again
	if resumeState.terms[handle] == tab.resume {
		delete(resumeState.terms, handle)
	}
	resumeState.mu.Unlock()
	state := tab.resume
	tab.resume = nil
	if discard {
		state.close()
	}
}

// resumeHandle takes a report of the Program Resume Protocol that a
// program sent to a terminal.
func resumeHandle(handle uintptr, body string) {
	resumeState.mu.RLock()
	state := resumeState.terms[handle]
	resumeState.mu.RUnlock()
	if state == nil {
		return
	}
	state.handle(body)
}

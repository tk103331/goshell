package main

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo/plugins/terminal"
)

// libghostty-vt keeps unknown OSC sequences for the embedder, and mygo's
// terminal does not expose that callback yet. The bridge below reaches the
// terminal's handle and the library's own options, which are ABI-stable:
// it makes the reports of the Program Status Protocol (OSC 7501) and of the
// Program Resume Protocol (OSC 7577) work in every terminal, local or
// remote, until mygo grows the hook itself. Every failure leaves the rest
// of the app alone.

// statusABI is the part of libghostty-vt's ghostty_type_json that the
// bridge reads: option numbers and the offsets of the unknown-sequence
// callback's structs.
type statusABI struct {
	Types map[string]statusABIType `json:"types"`
}

type statusABIType struct {
	Size   int                       `json:"size"`
	Values map[string]int32          `json:"values"`
	Fields map[string]statusABIField `json:"fields"`
}

type statusABIField struct {
	Offset int `json:"offset"`
	Size   int `json:"size"`
}

type statusLayout struct {
	unknownTag, unknownValue int
	oscTruncated, oscContent int
	oscTerminator            int
	stringPtr, stringLen     int
	promptKind               int

	optUnknown, optMax  int32
	optPrompt, optReset int32
}

var statusState struct {
	once sync.Once
	err  error

	setValue func(uintptr, int32, uintptr) int32
	setPtr   func(uintptr, int32, unsafe.Pointer) int32
	layout   statusLayout

	unknownCB, promptCB, resetCB uintptr

	mu    sync.RWMutex
	terms map[uintptr]*programStatus
}

// attachProgramStatus reports a terminal's unknown OSC sequences and its
// shell's prompt events to its status, when the library can.
func attachProgramStatus(tab *Tab) {
	if tab == nil || tab.term == nil || tab.status == nil {
		return
	}
	handle := terminalHandle(tab.term)
	if handle == 0 {
		return
	}
	statusState.once.Do(initProgramStatusBridge)
	if statusState.err != nil {
		return
	}

	statusState.mu.Lock()
	if statusState.terms == nil {
		statusState.terms = map[uintptr]*programStatus{}
	}
	statusState.terms[handle] = tab.status
	statusState.mu.Unlock()

	if !enableUnknownSequences(handle) {
		return
	}
	if statusState.layout.optPrompt != 0 {
		statusSet(handle, statusState.layout.optPrompt, statusState.promptCB)
	}
	if statusState.layout.optReset != 0 {
		statusSet(handle, statusState.layout.optReset, statusState.resetCB)
	}
}

// enableUnknownSequences gives a terminal the library's callback for the
// sequences it does not know, which the reports of the Program Status
// Protocol and of the Program Resume Protocol come through. It reports
// false when the library cannot say what it holds.
func enableUnknownSequences(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	statusState.once.Do(initProgramStatusBridge)
	if statusState.err != nil {
		return false
	}
	if !statusSetPtr(handle, statusState.layout.optMax, unsafe.Pointer(&maxStatusBytes)) {
		return false
	}
	statusSet(handle, statusState.layout.optUnknown, statusState.unknownCB)
	return true
}

// detachProgramStatus forgets a terminal being closed.
func detachProgramStatus(tab *Tab) {
	if tab == nil || tab.term == nil {
		return
	}
	handle := terminalHandle(tab.term)
	if handle == 0 {
		return
	}
	statusState.mu.Lock()
	delete(statusState.terms, handle)
	statusState.mu.Unlock()
}

// maxStatusBytes is the most of an unknown sequence the library keeps for
// the callback: the protocol's whole-sequence limit.
var maxStatusBytes uintptr = 4096

func statusSet(handle uintptr, opt int32, value uintptr) bool {
	if statusState.setValue == nil {
		return false
	}
	return statusState.setValue(handle, opt, value) == 0
}

func statusSetPtr(handle uintptr, opt int32, value unsafe.Pointer) bool {
	if statusState.setPtr == nil {
		return false
	}
	return statusState.setPtr(handle, opt, value) == 0
}

// statusOf finds the status a callback's terminal belongs to.
func statusOf(handle uintptr) *programStatus {
	statusState.mu.RLock()
	defer statusState.mu.RUnlock()
	return statusState.terms[handle]
}

func initProgramStatusBridge() {
	path, err := terminal.LibraryPath()
	if err != nil {
		statusState.err = err
		log.Println("program status:", err)
		return
	}
	lib, err := openStatusLibrary(path)
	if err != nil {
		statusState.err = err
		log.Println("program status:", err)
		return
	}
	setAddr, err := statusLibrarySymbol(lib, "ghostty_terminal_set")
	if err != nil {
		statusState.err = err
		log.Println("program status:", err)
		return
	}
	jsonAddr, err := statusLibrarySymbol(lib, "ghostty_type_json")
	if err != nil {
		statusState.err = err
		log.Println("program status:", err)
		return
	}

	var setValue func(uintptr, int32, uintptr) int32
	var setPtr func(uintptr, int32, unsafe.Pointer) int32
	purego.RegisterFunc(&setValue, setAddr)
	purego.RegisterFunc(&setPtr, setAddr)
	var typeJSON func() *byte
	purego.RegisterFunc(&typeJSON, jsonAddr)

	raw := cString(typeJSON())
	var abi statusABI
	if err := json.Unmarshal([]byte(raw), &abi); err != nil {
		statusState.err = err
		log.Println("program status:", err)
		return
	}
	layout, err := readStatusLayout(abi)
	if err != nil {
		statusState.err = err
		log.Println("program status:", err)
		return
	}

	statusState.setValue = setValue
	statusState.setPtr = setPtr
	statusState.layout = layout
	statusState.terms = map[uintptr]*programStatus{}
	statusState.unknownCB = purego.NewCallback(onUnknownSequence)
	statusState.promptCB = purego.NewCallback(onSemanticPrompt)
	statusState.resetCB = purego.NewCallback(onTerminalReset)
}

// readStatusLayout finds the options and struct offsets the bridge needs,
// or says that this library cannot report program status.
func readStatusLayout(abi statusABI) (statusLayout, error) {
	var l statusLayout
	opts, ok := abi.Types["GhosttyTerminalOption"]
	if !ok {
		return l, errors.New("libghostty-vt has no terminal options")
	}
	for _, it := range []struct {
		name string
		dst  *int32
	}{
		{"UNKNOWN_SEQUENCE", &l.optUnknown},
		{"UNKNOWN_MAX_BYTES", &l.optMax},
		{"SEMANTIC_PROMPT", &l.optPrompt},
		{"RESET", &l.optReset},
	} {
		v, ok := opts.Values[it.name]
		if !ok {
			if it.name == "SEMANTIC_PROMPT" || it.name == "RESET" {
				continue
			}
			return l, errors.New("libghostty-vt cannot report unknown OSC sequences")
		}
		*it.dst = v
	}

	unknown := abi.Types["GhosttyTerminalUnknownSequence"].Fields
	osc := abi.Types["GhosttyTerminalUnknownOscSequence"].Fields
	str := abi.Types["GhosttyString"].Fields
	prompt := abi.Types["GhosttyTerminalSemanticPrompt"].Fields
	var okTag, okValue, okTrunc, okContent, okTerm, okPtr, okLen bool
	l.unknownTag, okTag = unknown["tag"].Offset, hasField(unknown, "tag")
	l.unknownValue, okValue = unknown["value"].Offset, hasField(unknown, "value")
	l.oscTruncated, okTrunc = osc["truncated"].Offset, hasField(osc, "truncated")
	l.oscContent, okContent = osc["content"].Offset, hasField(osc, "content")
	l.oscTerminator, okTerm = osc["terminator"].Offset, hasField(osc, "terminator")
	l.stringPtr, okPtr = str["ptr"].Offset, hasField(str, "ptr")
	l.stringLen, okLen = str["len"].Offset, hasField(str, "len")
	if !okTag || !okValue || !okTrunc || !okContent || !okTerm || !okPtr || !okLen {
		return l, errors.New("libghostty-vt unknown sequence layout changed")
	}
	if l.optPrompt != 0 {
		if _, ok := prompt["kind"]; !ok {
			l.optPrompt = 0
		} else {
			l.promptKind = prompt["kind"].Offset
		}
	}
	return l, nil
}

func hasField(fields map[string]statusABIField, name string) bool {
	_, ok := fields[name]
	return ok
}

// onUnknownSequence gets every OSC and APC the library does not implement;
// only the reports of the Program Status Protocol (OSC 7501) and of the
// Program Resume Protocol (OSC 7577) are taken.
func onUnknownSequence(term, _ unsafe.Pointer, seq unsafe.Pointer) {
	defer func() {
		// a malformed report must not take the terminal down
		_ = recover()
	}()
	if seq == nil {
		return
	}
	l := statusState.layout
	if readI32(unsafe.Add(seq, l.unknownTag)) != 1 {
		return
	}
	value := unsafe.Add(seq, l.unknownValue)
	if readU8(unsafe.Add(value, l.oscTruncated)) != 0 {
		return
	}
	ptr, n := readString(unsafe.Add(value, l.oscContent), l)
	if ptr == nil || n == 0 || n > 4096 {
		return
	}
	content := string(unsafe.Slice((*byte)(ptr), n))
	// The Program Resume Protocol, OSC 7577, which the library does not
	// know either.
	if body, ok := strings.CutPrefix(content, "7577;"); ok {
		resumeHandle(uintptr(term), body)
		return
	}
	status := statusOf(uintptr(term))
	if status == nil {
		return
	}
	body, ok := strings.CutPrefix(content, "7501;")
	if !ok {
		return
	}
	if body == "?" {
		status.reply(readI32(unsafe.Add(value, l.oscTerminator)))
		return
	}
	rep, ok := parseProgramReport(body)
	if !ok {
		return
	}
	status.apply(rep)
}

// onSemanticPrompt clears working and blocked records when a shell draws a
// new prompt, as the protocol asks.
func onSemanticPrompt(term, _ unsafe.Pointer, event unsafe.Pointer) {
	defer func() { _ = recover() }()
	status := statusOf(uintptr(term))
	if status == nil || event == nil || statusState.layout.optPrompt == 0 {
		return
	}
	if readI32(unsafe.Add(event, statusState.layout.promptKind)) == 1 { // PROMPT_START
		status.clearActive()
	}
}

// onTerminalReset drops every record on a full reset.
func onTerminalReset(term, _ unsafe.Pointer) {
	defer func() { _ = recover() }()
	if status := statusOf(uintptr(term)); status != nil {
		status.reset()
	}
}

func readU8(p unsafe.Pointer) uint8 { return *(*uint8)(p) }

func readI32(p unsafe.Pointer) int32 { return *(*int32)(p) }

// readString reads a GhosttyString at p.
func readString(p unsafe.Pointer, l statusLayout) (unsafe.Pointer, uintptr) {
	ptr := *(*unsafe.Pointer)(unsafe.Add(p, l.stringPtr))
	n := *(*uintptr)(unsafe.Add(p, l.stringLen))
	return ptr, n
}

// terminalHandle finds the libghostty-vt handle mygo's terminal holds. The
// fields are private, so it reads them by reflection; a layout it does not
// know leaves status reporting off.
func terminalHandle(t *terminal.Terminal) uintptr {
	if t == nil {
		return 0
	}
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return 0
	}
	term := v.Elem().FieldByName("term")
	if !term.IsValid() || term.Kind() != reflect.Pointer || term.IsNil() {
		return 0
	}
	vt := reflect.NewAt(term.Type().Elem(), term.UnsafePointer()).Elem()
	h := vt.FieldByName("h")
	if !h.IsValid() {
		return 0
	}
	switch h.Kind() {
	case reflect.Uintptr, reflect.Uint:
		return uintptr(h.Uint())
	}
	return 0
}

// cString reads a NUL-terminated C string.
func cString(p *byte) string {
	if p == nil {
		return ""
	}
	b := unsafe.Slice(p, 1<<20)
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

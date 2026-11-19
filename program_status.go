package main

import (
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/plugins/terminal"
	"goshell/locales"
)

// The Program Status Protocol, OSC 7501: a program tells the terminal what
// it is doing, with a state, an optional kind, app, title, message and
// progress. The records of a terminal live beside it, one per id, and the
// view shows the most important one on the tab and under its terminal.
const (
	statusIdle    = "idle"
	statusWorking = "working"
	statusDone    = "done"
	statusBlocked = "blocked"
	statusError   = "error"
	statusClear   = "clear"

	statusPermission = "permission"
	statusQuestion   = "question"
	statusAuth       = "auth"

	statusMaxRecords = 256
)

// programRecord is one record a program reported, by its id.
type programRecord struct {
	id       string
	state    string
	kind     string
	app      string
	title    string
	msg      string
	progress int // -1 when absent
	seq      uint64
}

// programStatus holds the records of one terminal.
type programStatus struct {
	mu      sync.Mutex
	records map[string]*programRecord
	seq     uint64
	term    *terminal.Terminal
	changed func()
}

func newProgramStatus(term *terminal.Terminal, changed func()) *programStatus {
	return &programStatus{
		records: map[string]*programRecord{},
		term:    term,
		changed: changed,
	}
}

// programReport is a report that parsed and passed the protocol's limits.
type programReport struct {
	id       string
	state    string
	kind     string
	app      string
	title    string
	msg      string
	progress int
}

// parseProgramReport parses the body of an OSC 7501 report, after "7501;".
// It returns false when the report is discarded whole.
func parseProgramReport(body string) (programReport, bool) {
	rep := programReport{progress: -1}
	if len(body) > 4096 {
		return rep, false
	}

	vals := map[string]string{}
	for _, pair := range strings.Split(body, ":") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(key) > 16 {
			// a limit, not a malformed pair the rest survives
			return rep, false
		}
		if !validStatusKey(key) || !validStatusValue(value) {
			continue
		}
		vals[key] = value
	}

	state := vals["state"]
	switch state {
	case statusIdle, statusWorking, statusDone, statusBlocked, statusError, statusClear:
	default:
		return rep, false
	}
	rep.state = state

	id := vals["id"]
	if !validStatusID(id) {
		return rep, false
	}
	rep.id = id

	if app := vals["app"]; app != "" {
		if len(app) > 32 {
			return rep, false
		}
		if validStatusSegment(app) {
			rep.app = app
		}
	}

	if title, ok := vals["title"]; ok {
		text, ok := decodeStatusText(title, 256, 192)
		if !ok {
			return rep, false
		}
		rep.title = text
	}
	if msg, ok := vals["msg"]; ok {
		text, ok := decodeStatusText(msg, 2732, 2048)
		if !ok {
			return rep, false
		}
		rep.msg = text
	}

	if state == statusBlocked {
		// unrecognized kinds are absent, not a reason to drop the report
		switch vals["kind"] {
		case statusPermission, statusQuestion, statusAuth:
			rep.kind = vals["kind"]
		}
	}
	if state == statusWorking || state == statusBlocked {
		if progress, ok := parseStatusProgress(vals["progress"]); ok {
			rep.progress = progress
		}
	}

	return rep, true
}

func validStatusKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func validStatusValue(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_.,+/=-", r):
		default:
			return false
		}
	}
	return true
}

func validStatusID(id string) bool {
	if id == "" {
		return true
	}
	if len(id) > 128 {
		return false
	}
	segments := strings.Split(id, "/")
	if len(segments) > 8 {
		return false
	}
	for _, s := range segments {
		if len(s) == 0 || len(s) > 32 || !validStatusSegment(s) {
			return false
		}
	}
	return true
}

func validStatusSegment(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_.+-", r):
		default:
			return false
		}
	}
	return true
}

// decodeStatusText decodes a base64 msg or title, whose padding the
// protocol says is optional, and refuses control characters.
func decodeStatusText(encoded string, maxEncoded, maxDecoded int) (string, bool) {
	if len(encoded) > maxEncoded {
		return "", false
	}
	if encoded == "" {
		return "", true
	}
	if pad := len(encoded) % 4; pad != 0 {
		encoded += strings.Repeat("=", 4-pad)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) > maxDecoded {
		return "", false
	}
	if !utf8.Valid(data) {
		return "", false
	}
	text := string(data)
	for _, r := range text {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return "", false
		}
	}
	return text, true
}

func parseStatusProgress(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 100 {
		return 0, false
	}
	return n, true
}

// apply puts a report into the terminal's records, replacing its record
// completely, or clears the record and those beneath it.
func (s *programStatus) apply(rep programReport) {
	s.mu.Lock()
	s.seq++
	changed := false
	if rep.state == statusClear {
		if rep.id == "" {
			s.records = map[string]*programRecord{}
			changed = true
		} else {
			changed = s.clearLocked(rep.id)
		}
	} else {
		if _, exists := s.records[rep.id]; !exists && len(s.records) >= statusMaxRecords {
			s.evictLocked()
		}
		s.records[rep.id] = &programRecord{
			id:       rep.id,
			state:    rep.state,
			kind:     rep.kind,
			app:      rep.app,
			title:    rep.title,
			msg:      rep.msg,
			progress: rep.progress,
			seq:      s.seq,
		}
		changed = true
	}
	s.mu.Unlock()
	if changed && s.changed != nil {
		s.changed()
	}
}

// clearLocked removes id and every record beneath it; s.mu is held.
func (s *programStatus) clearLocked(id string) bool {
	changed := false
	prefix := id + "/"
	for key := range s.records {
		if key == id || strings.HasPrefix(key, prefix) {
			delete(s.records, key)
			changed = true
		}
	}
	return changed
}

// evictLocked removes the record updated longest ago; s.mu is held.
func (s *programStatus) evictLocked() {
	var oldest string
	var seq uint64
	for id, rec := range s.records {
		if oldest == "" || rec.seq < seq {
			oldest, seq = id, rec.seq
		}
	}
	if oldest != "" {
		delete(s.records, oldest)
	}
}

// clearActive drops working and blocked records, as a new shell prompt
// does; done and error stay for the user to find.
func (s *programStatus) clearActive() {
	s.clearStates(statusWorking, statusBlocked)
}

// markSeen drops finished and failed records, as the user returns to the
// terminal and types.
func (s *programStatus) markSeen() {
	s.clearStates(statusDone, statusError)
}

func (s *programStatus) clearStates(states ...string) {
	drop := map[string]bool{}
	for _, state := range states {
		drop[state] = true
	}
	s.mu.Lock()
	changed := false
	for id, rec := range s.records {
		if drop[rec.state] {
			delete(s.records, id)
			changed = true
		}
	}
	s.mu.Unlock()
	if changed && s.changed != nil {
		s.changed()
	}
}

// reset drops every record, as a full terminal reset does.
func (s *programStatus) reset() {
	s.mu.Lock()
	changed := len(s.records) > 0
	s.records = map[string]*programRecord{}
	s.mu.Unlock()
	if changed && s.changed != nil {
		s.changed()
	}
}

// summary is the record the tab and its status bar show: blocked first,
// then error, done and working, the latest of each rank.
func (s *programStatus) summary() (programRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best programRecord
	bestRank := -1
	for _, rec := range s.records {
		r := statusRank(rec.state)
		if r < 0 {
			continue
		}
		if r > bestRank || (r == bestRank && rec.seq > best.seq) {
			best = *rec
			bestRank = r
		}
	}
	if bestRank < 0 {
		return programRecord{}, false
	}
	best.app = s.appOfLocked(best)
	return best, true
}

// statusRank is how much attention a record asks for, -1 for a state the
// view does not rank.
func statusRank(state string) int {
	switch state {
	case statusBlocked:
		return 4
	case statusError:
		return 3
	case statusDone:
		return 2
	case statusWorking:
		return 1
	case statusIdle:
		return 0
	}
	return -1
}

// appOfLocked inherits app from the nearest ancestor that has one; s.mu is
// held.
func (s *programStatus) appOfLocked(rec programRecord) string {
	if rec.app != "" {
		return rec.app
	}
	id := rec.id
	for {
		i := strings.LastIndex(id, "/")
		if i < 0 {
			break
		}
		id = id[:i]
		if parent := s.records[id]; parent != nil && parent.app != "" {
			return parent.app
		}
	}
	if root := s.records[""]; root != nil {
		return root.app
	}
	return ""
}

// statusText is the one-line description of a record for a tab or a row,
// with invisible formatting characters taken out.
func statusText(rec programRecord) string {
	var b strings.Builder
	b.WriteString(statusLabel(rec.state))
	if name := statusName(rec); name != "" {
		b.WriteString(" · ")
		b.WriteString(name)
	}
	if rec.progress >= 0 {
		b.WriteString(" ")
		b.WriteString(strconv.Itoa(rec.progress))
		b.WriteString("%")
	}
	if rec.msg != "" {
		b.WriteString(" — ")
		b.WriteString(sanitizeStatus(rec.msg))
	}
	return b.String()
}

// statusName is the label of a record: its app, else its title.
func statusName(rec programRecord) string {
	if rec.app != "" {
		return sanitizeStatus(rec.app)
	}
	return sanitizeStatus(rec.title)
}

// statusLabel names a state in the app's language.
func statusLabel(state string) string {
	switch state {
	case statusBlocked:
		return locales.ProgramStatusBlocked()
	case statusWorking:
		return locales.ProgramStatusWorking()
	case statusDone:
		return locales.ProgramStatusDone()
	case statusError:
		return locales.ProgramStatusError()
	}
	return state
}

// sanitizeStatus removes the invisible formatting characters the protocol
// says to take out of text shown outside the terminal grid.
func sanitizeStatus(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

// reply answers the protocol's feature detection with the states and kinds
// the app supports, ending as the query did.
func (s *programStatus) reply(terminator int32) {
	if s.term == nil {
		return
	}
	end := "\x1b\\"
	if terminator == 1 { // BEL
		end = "\a"
	}
	s.term.Send([]byte("\x1b]7501;?:states=idle,working,done,blocked,error:kinds=permission,question,auth" + end))
}

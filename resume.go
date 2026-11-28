package main

// The Program Resume Protocol, OSC 7577: a program tells the terminal how
// to bring it back after the terminal restarts. The app gives each of its
// terminals a token, in the environment of the first process on its
// pseudo-terminal, and keeps the reports whose hash the token proves: the
// command to run, the directory to run it in, and the program that owns it.
// The records outlive the app, so that a program the user was working in
// can come back after a restart.
//
// The protocol: https://www.superlogical.com/rex/docs/build/program-resume
import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// resumeTokenEnv is the environment variable the terminal sets for the
	// first process on its pseudo-terminal. A program that finds it knows
	// the terminal speaks the protocol; there is no query and no reply.
	resumeTokenEnv = "OSC7577_TOKEN"

	// resumeMaxLength is the most of a report the protocol allows: the
	// whole sequence, with its prefix and its terminator.
	resumeMaxLength = 2048

	// resumeMaxArgv is the most arguments a report may carry.
	resumeMaxArgv = 64

	// resumeTimeWindow is how far a report's time may be from the app's
	// own clock, in seconds. A report older than that is a recording, and
	// a recording must not register a command.
	resumeTimeWindow = 60
)

// resumeKeys are the keys the app implements. A report that asks for
// another one is rejected, as the protocol says.
var resumeKeys = map[string]bool{
	"pid":   true,
	"time":  true,
	"argv":  true,
	"cwd":   true,
	"clear": true,
	"need":  true,
	"mac":   true,
}

// resumeReport is what a report says: the program that owns it, when it
// was made, and the command that brings it back.
type resumeReport struct {
	PID   int
	Time  int64
	Argv  []string
	Cwd   string
	Clear bool
}

// parseResumeReport parses the body of a report, the part after "7577;".
// It reports false when the report is malformed, which rejects it whole:
// the protocol says the terminal drops the records of the program that sent
// it, whose command has changed.
func parseResumeReport(body string) (resumeReport, bool) {
	var rep resumeReport
	if body == "" || len(body) > resumeMaxLength {
		return rep, false
	}
	pairs := strings.Split(body, ":")
	// The mac is the last key, and its hash covers the rest of the body.
	mac, ok := strings.CutPrefix(pairs[len(pairs)-1], "mac=")
	if !ok || !validResumeMac(mac) {
		return rep, false
	}
	values := map[string]string{}
	for _, pair := range pairs[:len(pairs)-1] {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || !validResumeKey(key) || !validResumeValue(value) {
			return rep, false
		}
		if _, dup := values[key]; dup {
			return rep, false
		}
		values[key] = value
	}

	pid, ok := resumeNumber(values["pid"])
	if !ok {
		return rep, false
	}
	rep.PID = int(pid)
	tm, ok := resumeNumber(values["time"])
	if !ok {
		return rep, false
	}
	rep.Time = tm

	// need names the keys the terminal has to implement: a report that
	// asks for one it does not know is rejected.
	if need, ok := values["need"]; ok && need != "" {
		for _, key := range strings.Split(need, ",") {
			if !resumeKeys[key] {
				return rep, false
			}
		}
	}

	// A report has exactly one of argv and clear.
	argv, hasArgv := values["argv"]
	clear, hasClear := values["clear"]
	if hasArgv == hasClear {
		return rep, false
	}
	if hasClear {
		if clear != "1" {
			return rep, false
		}
		rep.Clear = true
		return rep, true
	}

	elems := strings.Split(argv, ",")
	if len(elems) > resumeMaxArgv {
		return rep, false
	}
	args := make([]string, 0, len(elems))
	for i, elem := range elems {
		text, ok := decodeResumeText(elem)
		if !ok {
			return rep, false
		}
		if i == 0 && text == "" {
			// the first element is the program to run
			return rep, false
		}
		args = append(args, text)
	}
	if !validResumeProgram(args[0]) {
		return rep, false
	}
	rep.Argv = args

	if cwd, ok := values["cwd"]; ok {
		dir, ok := decodeResumeText(cwd)
		if !ok || !filepath.IsAbs(dir) {
			return rep, false
		}
		rep.Cwd = dir
	}
	return rep, true
}

// validResumeKey says whether a key is one the protocol names: lower case
// letters and nothing else.
func validResumeKey(key string) bool {
	if key == "" || len(key) > 16 {
		return false
	}
	for _, r := range key {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// validResumeValue says whether a value is one the protocol allows: the
// characters of base64 and the few others the keys use. The colon and the
// semicolon are not among them, so nothing needs escaping.
func validResumeValue(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_,./+-=", r):
		default:
			return false
		}
	}
	return true
}

// validResumeMac says whether a hash is one the protocol allows: 64 lower
// case hexadecimal characters.
func validResumeMac(mac string) bool {
	if len(mac) != 64 {
		return false
	}
	for _, r := range mac {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// resumeNumber reads a number of a report: digits, and no leading zero.
func resumeNumber(value string) (int64, bool) {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// validResumeProgram says whether the first element of a command is a
// program the terminal may run: a bare name, which is looked up on PATH,
// or an absolute path. A relative path is not allowed, and a bare name may
// not start with a dash.
func validResumeProgram(name string) bool {
	if name == "" {
		return false
	}
	if filepath.IsAbs(name) {
		return true
	}
	if len(name) > 2 && name[1] == ':' && (name[2] == '\\' || name[2] == '/') {
		return true // an absolute path on Windows
	}
	if strings.ContainsAny(name, `/\`) {
		return false // a relative path
	}
	return !strings.HasPrefix(name, "-")
}

// decodeResumeText decodes a base64 text of a report, whose padding the
// protocol says is optional, and refuses what may not be shown: bytes that
// are not text, and control characters.
func decodeResumeText(encoded string) (string, bool) {
	if encoded == "" {
		return "", true
	}
	if pad := len(encoded) % 4; pad != 0 {
		encoded += strings.Repeat("=", 4-pad)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
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

// resumeMac returns the hash a report carries: the SHA-256 of the body
// before its ":mac=", a colon, and the token. The token comes last, so
// that knowing one hash does not give the hash of a longer body.
func resumeMac(body, token string) string {
	sum := sha256.Sum256([]byte(body + ":" + token))
	return hex.EncodeToString(sum[:])
}

// checkResumeMac checks the hash of a report. A report that fails is
// ignored whole: the terminal cannot know who sent it, so it changes
// nothing, not even the records of the program it names.
func checkResumeMac(body, token string) bool {
	i := strings.LastIndex(body, ":mac=")
	if i < 0 {
		return false
	}
	want := resumeMac(body[:i], token)
	return hmac.Equal([]byte(want), []byte(body[i+len(":mac="):]))
}

// resumeReportTime reads the time of a report, before the body is parsed
// strictly: the protocol checks the time before anything else, and a
// report without one is ignored.
func resumeReportTime(body string) int64 {
	for _, pair := range strings.Split(body, ":") {
		if value, ok := strings.CutPrefix(pair, "time="); ok {
			tm, ok := resumeNumber(value)
			if !ok {
				return 0
			}
			return tm
		}
	}
	return 0
}

// resumeReportPID reads the program a report is about, leniently: a report
// that does not parse still names a program, whose records are dropped
// when the report is rejected.
func resumeReportPID(body string) int {
	for _, pair := range strings.Split(body, ":") {
		if value, ok := strings.CutPrefix(pair, "pid="); ok {
			pid, ok := resumeNumber(value)
			if !ok {
				return 0
			}
			return int(pid)
		}
	}
	return 0
}

// resumeTimeOK says whether a report's time is near the app's own clock.
func resumeTimeOK(reportTime int64) bool {
	now := time.Now().Unix()
	diff := now - reportTime
	if diff < 0 {
		diff = -diff
	}
	return diff <= resumeTimeWindow
}

// newResumeToken returns the token of a terminal: 32 random characters of
// the alphabet the protocol allows, which is more than the 22 it asks for.
// A terminal has a token of its own, and a new one every time it starts;
// two panes never share one.
func newResumeToken() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on the platforms the app runs on
		log.Println("resume token:", err)
		return ""
	}
	for i, v := range b {
		b[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(b)
}

// resumeRecord is what the app keeps of a program that can be resumed: the
// command to run, the directory to run it in, and the program that owns it.
type resumeRecord struct {
	PID   int      `json:"pid"`
	Argv  []string `json:"argv"`
	Cwd   string   `json:"cwd,omitempty"`
	Time  int64    `json:"time"`
	Saved int64    `json:"saved"`
	UID   int      `json:"uid"`
	Start string   `json:"start,omitempty"`
}

// resumeStore keeps the records of the programs that can be resumed, and
// saves them in the app's data directory: they outlive the app, so that a
// program can come back after a restart. The file holds commands, which
// are as sensitive as a shell's startup files, so only the user may read
// it.
type resumeStore struct {
	mu      sync.Mutex
	saveMu  sync.Mutex
	writes  sync.WaitGroup
	path    string
	records map[int]*resumeRecord
	changed func()
}

// openResumeStore opens the store the app keeps its records in, and reads
// what the last run left there.
func openResumeStore(path string) *resumeStore {
	s := &resumeStore{path: path, records: map[int]*resumeRecord{}}
	s.load()
	return s
}

// load reads the records of the last run.
func (s *resumeStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var records []*resumeRecord
	if err := json.Unmarshal(data, &records); err != nil {
		log.Println("resume records:", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range records {
		if rec == nil || rec.PID < 1 || len(rec.Argv) == 0 {
			continue
		}
		s.records[rec.PID] = rec
	}
}

// save writes the records down, as they change: a program that reports
// again, or a record that goes, is written at once.
func (s *resumeStore) save() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	records := make([]*resumeRecord, 0, len(s.records))
	for _, rec := range s.records {
		records = append(records, rec)
	}
	s.mu.Unlock()
	sort.Slice(records, func(i, j int) bool { return records[i].PID < records[j].PID })
	data, err := json.Marshal(records)
	if err != nil {
		log.Println("resume records:", err)
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*.tmp")
	if err != nil {
		log.Println("resume records:", err)
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		log.Println("resume records:", err)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		log.Println("resume records:", err)
		return
	}
	// the file holds commands: only the user may read it
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		log.Println("resume records:", err)
		return
	}
	if err := os.Rename(name, s.path); err != nil {
		log.Println("resume records:", err)
		os.Remove(name)
	}
}

// saveAsync writes the records down in the background: the reports come
// from a terminal's own thread, which must not wait for the disk.
func (s *resumeStore) saveAsync() {
	s.writes.Add(1)
	go func() {
		defer s.writes.Done()
		s.save()
	}()
}

// flush waits for the writes in flight, as the app does when it quits.
func (s *resumeStore) flush() {
	s.writes.Wait()
	s.save()
}

// accept keeps the record of a program that reported how to resume it.
func (s *resumeStore) accept(rec *resumeRecord) {
	if rec == nil || rec.PID < 1 || len(rec.Argv) == 0 {
		return
	}
	s.mu.Lock()
	s.records[rec.PID] = rec
	s.mu.Unlock()
	s.saveAsync()
	s.notify()
}

// remove drops the record of a program: it said it should not be resumed,
// its terminal was closed, or it was brought back.
func (s *resumeStore) remove(pid int) {
	s.mu.Lock()
	_, ok := s.records[pid]
	delete(s.records, pid)
	s.mu.Unlock()
	if !ok {
		return
	}
	s.saveAsync()
	s.notify()
}

// list returns the records the store keeps.
func (s *resumeStore) list() []*resumeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]*resumeRecord, 0, len(s.records))
	for _, rec := range s.records {
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].PID < records[j].PID })
	return records
}

// notify asks for a frame, as the records changed.
func (s *resumeStore) notify() {
	if s.changed != nil {
		s.changed()
	}
}

// resumable returns the records of the programs that are gone, and so can
// be brought back.
func (s *resumeStore) resumable() []*resumeRecord {
	var out []*resumeRecord
	for _, rec := range s.list() {
		if canResumeRecord(rec) {
			out = append(out, rec)
		}
	}
	return out
}

// canResumeRecord says whether a record may be used: the program must have
// run as the user the app runs as, and the process it named must not be
// another program that took its place.
func canResumeRecord(rec *resumeRecord) bool {
	if rec == nil {
		return false
	}
	if rec.UID >= 0 && rec.UID != os.Getuid() {
		// another user's program: the terminal must not run it again
		return false
	}
	st, ok := lookupProcess(rec.PID)
	if !ok {
		// the system does not say: the program is most likely gone
		return true
	}
	if !st.alive {
		return true
	}
	if rec.Start != "" && st.start != "" && rec.Start != st.start {
		// another program took the pid: the one that reported is gone
		return true
	}
	// the program is still running: it is not something to bring back
	return false
}

// resumeTerminal is what one terminal keeps of the reports it received: the
// token its programs hold, and the programs that reported to it.
type resumeTerminal struct {
	token string
	store *resumeStore

	mu   sync.Mutex
	pids map[int]bool  // the programs that reported to this terminal
	last map[int]int64 // the time of the last report of each
}

// handle takes a report a program sent to the terminal. The protocol
// verifies the hash and the time before anything else, and ignores a
// report that fails either check; a report that parses badly is rejected,
// which drops the records of the program that sent it.
func (t *resumeTerminal) handle(body string) {
	if t == nil || t.token == "" || t.store == nil {
		return
	}
	if !checkResumeMac(body, t.token) {
		return
	}
	tm := resumeReportTime(body)
	if tm == 0 || !resumeTimeOK(tm) {
		return
	}
	rep, ok := parseResumeReport(body)
	if !ok {
		t.reject(body)
		return
	}
	// The protocol asks the terminal to check the program the report is
	// about: it should be running, and it should run as the user the app
	// runs as.
	st, known := lookupProcess(rep.PID)
	if known {
		if !st.alive {
			t.reject(body)
			return
		}
		if st.uid >= 0 && st.uid != os.Getuid() {
			t.reject(body)
			return
		}
	}
	// A report that is older than the last one the program sent does not
	// replace it.
	t.mu.Lock()
	last, seen := t.last[rep.PID]
	if seen && rep.Time < last {
		t.mu.Unlock()
		return
	}
	t.last[rep.PID] = rep.Time
	t.pids[rep.PID] = true
	t.mu.Unlock()

	if rep.Clear {
		t.forget(rep.PID)
		t.store.remove(rep.PID)
		return
	}
	rec := &resumeRecord{
		PID:   rep.PID,
		Argv:  rep.Argv,
		Cwd:   rep.Cwd,
		Time:  rep.Time,
		Saved: time.Now().Unix(),
		UID:   os.Getuid(),
	}
	if known {
		rec.Start = st.start
	}
	t.store.accept(rec)
}

// reject drops the records of the program a report was about: the protocol
// says a rejected report still means the program's command has changed.
func (t *resumeTerminal) reject(body string) {
	pid := resumeReportPID(body)
	if pid < 1 {
		// a report with no usable pid has no owner
		return
	}
	t.forget(pid)
	t.store.remove(pid)
}

// forget drops what the terminal kept of a program.
func (t *resumeTerminal) forget(pid int) {
	t.mu.Lock()
	delete(t.pids, pid)
	delete(t.last, pid)
	t.mu.Unlock()
}

// close drops the records of the programs that reported to the terminal:
// the protocol says a terminal drops them when the user closes it on
// purpose.
func (t *resumeTerminal) close() {
	t.mu.Lock()
	pids := make([]int, 0, len(t.pids))
	for pid := range t.pids {
		pids = append(pids, pid)
	}
	t.pids = map[int]bool{}
	t.last = map[int]int64{}
	t.mu.Unlock()
	if t.store == nil {
		return
	}
	for _, pid := range pids {
		t.store.remove(pid)
	}
}

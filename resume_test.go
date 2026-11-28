package main

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

// resumeTestToken is the token the protocol's own examples use.
const resumeTestToken = "dummy-token-replace-me"

// resumeBody builds the body of a report, with the hash that proves the
// sender knows the token.
func resumeBody(body, token string) string {
	return body + ":mac=" + resumeMac(body, token)
}

// TestResumeMac checks the hash against the examples of the protocol, whose
// hashes are the real ones.
func TestResumeMac(t *testing.T) {
	for _, it := range []struct {
		body string
		mac  string
	}{
		{
			"pid=74877:time=1795543200:clear=1",
			"c30603369f98fef581a4098be889175b0c74f0641e49f8b25f4326cb38f97c93",
		},
		{
			"pid=74877:time=1795543200:argv=bnZpbQ==,LVM=,L2hvbWUvdXNlci8ubG9jYWwvc3RhdGUvbnZpbS9zZXNzaW9ucy84ZjJjMWQudmlt",
			"9dbb99bfb856749a87563839d7ce9062818d78cf25909db93f309e0cd3b1909b",
		},
		{
			"pid=74877:time=1795543200:argv=bGF6eWdpdA==:cwd=L2hvbWUvdXNlci9zcmMvcHJvamVjdA==",
			"35cb909805c380336efd3689a125acc774a9dec8422a1d21492ebec06ca758e1",
		},
		{
			"pid=90412:time=1795543200:argv=cmVzdW1lLXdyYXA=,c3No,ZGV2Ym94",
			"e76149d0ba54a83570ffab0583867dacdfa23928ba9d18604f0badea88b62b65",
		},
	} {
		if got := resumeMac(it.body, resumeTestToken); got != it.mac {
			t.Errorf("resumeMac(%q) = %s, want %s", it.body, got, it.mac)
		}
		if !checkResumeMac(it.body+":mac="+it.mac, resumeTestToken) {
			t.Errorf("checkResumeMac(%q) is false", it.body)
		}
		// another token gives another hash
		if checkResumeMac(it.body+":mac="+it.mac, "another-token") {
			t.Errorf("checkResumeMac(%q) is true with another token", it.body)
		}
	}
}

// TestParseResumeReport checks the reports the protocol accepts.
func TestParseResumeReport(t *testing.T) {
	rep, ok := parseResumeReport("pid=74877:time=1795543200:argv=bnZpbQ==,LVM=,L2hvbWUvdXNlci8ubG9jYWwvc3RhdGUvbnZpbS9zZXNzaW9ucy84ZjJjMWQudmlt:mac=9dbb99bfb856749a87563839d7ce9062818d78cf25909db93f309e0cd3b1909b")
	if !ok {
		t.Fatal("the report of the protocol was rejected")
	}
	if rep.PID != 74877 || rep.Time != 1795543200 {
		t.Errorf("pid %d, time %d", rep.PID, rep.Time)
	}
	want := []string{"nvim", "-S", "/home/user/.local/state/nvim/sessions/8f2c1d.vim"}
	if len(rep.Argv) != len(want) {
		t.Fatalf("argv %q, want %q", rep.Argv, want)
	}
	for i, arg := range want {
		if rep.Argv[i] != arg {
			t.Errorf("argv %q, want %q", rep.Argv, want)
		}
	}

	rep, ok = parseResumeReport("pid=74877:time=1795543200:argv=bGF6eWdpdA==:cwd=L2hvbWUvdXNlci9zcmMvcHJvamVjdA==:mac=35cb909805c380336efd3689a125acc774a9dec8422a1d21492ebec06ca758e1")
	if !ok {
		t.Fatal("the report with a directory was rejected")
	}
	if rep.Cwd != "/home/user/src/project" {
		t.Errorf("cwd %q", rep.Cwd)
	}
	if len(rep.Argv) != 1 || rep.Argv[0] != "lazygit" {
		t.Errorf("argv %q", rep.Argv)
	}

	rep, ok = parseResumeReport("pid=74877:time=1795543200:clear=1:mac=c30603369f98fef581a4098be889175b0c74f0641e49f8b25f4326cb38f97c93")
	if !ok {
		t.Fatal("the report that clears was rejected")
	}
	if !rep.Clear {
		t.Error("the report that clears was not a clear")
	}
}

// TestParseResumeReportRejects checks the reports the protocol rejects.
func TestParseResumeReportRejects(t *testing.T) {
	for _, body := range []string{
		// the mac is not a hash of the protocol
		"pid=74877:time=1795543200:argv=bnZpbQ==:mac=0000",
		"pid=74877:time=1795543200:argv=bnZpbQ==:mac=000000000000000000000000000000000000000000000000000000000000000g",
		// there is no mac
		"pid=74877:time=1795543200:argv=bnZpbQ==",
		// the mac is not the last key
		"mac=0000000000000000000000000000000000000000000000000000000000000000:pid=74877:time=1795543200:argv=bnZpbQ==",
		// there is no time to check
		"pid=74877:argv=bnZpbQ==:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// the program is a relative path
		"pid=74877:time=1795543200:argv=Li9ydW4uc2g=:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// a report has exactly one of argv and clear
		"pid=74877:time=1795543200:argv=bnZpbQ==:clear=1:mac=0000000000000000000000000000000000000000000000000000000000000000",
		"pid=74877:time=1795543200:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// the directory is a relative path
		"pid=74877:time=1795543200:argv=bnZpbQ==:cwd=cHJvamVjdA==:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// a key repeats
		"pid=74877:pid=74878:time=1795543200:argv=bnZpbQ==:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// a pair is malformed
		"pid=74877:time=1795543200:argv:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// the program may not start with a dash
		"pid=74877:time=1795543200:argv=LW12aW0=:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// the program may not be empty
		"pid=74877:time=1795543200:argv=:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// the key names a feature the terminal does not have
		"pid=74877:time=1795543200:argv=bnZpbQ==:need=later:mac=0000000000000000000000000000000000000000000000000000000000000000",
		// the pid has a leading zero
		"pid=074877:time=1795543200:argv=bnZpbQ==:mac=0000000000000000000000000000000000000000000000000000000000000000",
	} {
		if _, ok := parseResumeReport(body); ok {
			t.Errorf("the report was taken: %q", body)
		}
	}
}

// TestParseResumeReportNeed checks the keys a report asks for: the ones the
// app has are taken, the others are not.
func TestParseResumeReportNeed(t *testing.T) {
	body := "pid=74877:time=1795543200:argv=bnZpbQ==:need=cwd,clear:mac=0000000000000000000000000000000000000000000000000000000000000000"
	if _, ok := parseResumeReport(body); !ok {
		t.Error("the report was rejected")
	}
	body = "pid=74877:time=1795543200:argv=bnZpbQ==:need=later:mac=0000000000000000000000000000000000000000000000000000000000000000"
	if _, ok := parseResumeReport(body); ok {
		t.Error("the report was taken")
	}
}

// TestResumeTime checks the window a report's time may be in.
func TestResumeTime(t *testing.T) {
	now := time.Now().Unix()
	if !resumeTimeOK(now) {
		t.Error("a report of now was refused")
	}
	if !resumeTimeOK(now - resumeTimeWindow) {
		t.Error("a report of the window's edge was refused")
	}
	if resumeTimeOK(now - resumeTimeWindow - 1) {
		t.Error("a report older than the window was taken")
	}
	if resumeTimeOK(now + resumeTimeWindow + 1) {
		t.Error("a report of the future was taken")
	}
}

// TestResumeToken checks the token of a terminal.
func TestResumeToken(t *testing.T) {
	token := newResumeToken()
	if len(token) < 22 || len(token) > 64 {
		t.Errorf("the token is %d characters long", len(token))
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			t.Errorf("the token holds %q", r)
		}
	}
	if token == newResumeToken() {
		t.Error("two tokens are the same")
	}
}

// TestResumeHandle checks what the terminal does with the reports it
// receives.
func TestResumeHandle(t *testing.T) {
	dir := t.TempDir()
	store := openResumeStore(filepath.Join(dir, "resume.json"))
	token := newResumeToken()
	term := &resumeTerminal{token: token, store: store, pids: map[int]bool{}, last: map[int]int64{}}
	pid := os.Getpid()
	now := strconv.FormatInt(time.Now().Unix(), 10)
	defer store.flush()

	// a report of a program that is running is kept
	argv := base64.StdEncoding.EncodeToString([]byte("nvim"))
	body := resumeBody("pid="+strconv.Itoa(pid)+":time="+now+":argv="+argv, token)
	term.handle(body)
	records := store.list()
	if len(records) != 1 {
		t.Fatalf("the store holds %d records", len(records))
	}
	if records[0].PID != pid || len(records[0].Argv) != 1 || records[0].Argv[0] != "nvim" {
		t.Errorf("the record is %+v", records[0])
	}

	// a report with another hash is ignored whole
	body = "pid=" + strconv.Itoa(pid) + ":time=" + now + ":argv=" + argv + ":mac=" + strings.Repeat("0", 64)
	term.handle(body)
	if len(store.list()) != 1 {
		t.Error("a report with a wrong hash was taken")
	}

	// a report of another time is ignored
	body = resumeBody("pid="+strconv.Itoa(pid)+":time=1000000000:argv="+argv, token)
	term.handle(body)
	if len(store.list()) != 1 {
		t.Error("a report of another time was taken")
	}

	// a report that clears the record drops it
	body = resumeBody("pid="+strconv.Itoa(pid)+":time="+now+":clear=1", token)
	term.handle(body)
	if len(store.list()) != 0 {
		t.Error("the record was not dropped")
	}
}

// TestResumeStore checks that the records outlive the app.
func TestResumeStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	store := openResumeStore(path)
	defer store.flush()
	store.accept(&resumeRecord{PID: 42, Argv: []string{"nvim", "-S", "/tmp/session"}, Cwd: "/tmp"})
	store.save()

	again := openResumeStore(path)
	records := again.list()
	if len(records) != 1 {
		t.Fatalf("the store holds %d records", len(records))
	}
	if records[0].PID != 42 || records[0].Cwd != "/tmp" {
		t.Errorf("the record is %+v", records[0])
	}
	if len(records[0].Argv) != 3 || records[0].Argv[1] != "-S" {
		t.Errorf("the command is %q", records[0].Argv)
	}

	// the file holds commands, which are as sensitive as a shell's startup
	// files: only the user may read it
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v", info.Mode().Perm())
	}

	again.remove(42)
	again.flush()
	if len(openResumeStore(path).list()) != 0 {
		t.Error("the record was not dropped")
	}
}

// resumeTestConn is a terminal fed by a pipe.
type resumeTestConn struct {
	io.Reader
	io.Writer
}

func (resumeTestConn) Close() error { return nil }

// TestResumeTerminalReport checks that a report a program sends reaches the
// app: the library keeps the OSC sequences it does not know, and the bridge
// takes them.
func TestResumeTerminalReport(t *testing.T) {
	r, pw := io.Pipe()
	term, err := terminal.New(terminal.Options{Conn: resumeTestConn{r, pw}})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()

	dir := t.TempDir()
	store := openResumeStore(filepath.Join(dir, "resume.json"))
	defer store.flush()
	token := newResumeToken()
	tab := &Tab{name: "local", term: term}
	attachResume(tab, token, store)
	defer detachResume(tab, false)

	pid := os.Getpid()
	argv := base64.StdEncoding.EncodeToString([]byte("nvim"))
	body := "pid=" + strconv.Itoa(pid) + ":time=" + strconv.FormatInt(time.Now().Unix(), 10) + ":argv=" + argv
	go pw.Write([]byte("\x1b]7577;" + resumeBody(body, token) + "\x1b\\"))

	var records []*resumeRecord
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		records = store.list()
		if len(records) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(records) != 1 {
		t.Fatalf("the store holds %d records", len(records))
	}
	if records[0].PID != pid || len(records[0].Argv) != 1 || records[0].Argv[0] != "nvim" {
		t.Errorf("the record is %+v", records[0])
	}
}

// TestResumeProcessReport checks the protocol from a program's side: a
// program that the terminal starts reads its token from the environment,
// reports how to bring it back, and the report reaches the store through
// the library, with the directory the program runs in.
func TestResumeProcessReport(t *testing.T) {
	if os.Getenv("GOSHELL_RESUME_TEST_CHILD") == "1" {
		resumeTestChild()
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store := openResumeStore(filepath.Join(dir, "resume.json"))
	defer store.flush()
	token := newResumeToken()

	// the test binary runs itself as the program of the terminal, and the
	// token goes in the environment of the first process on the pty
	term, err := terminal.New(terminal.Options{
		Command:       []string{exe, "-test.run=^TestResumeProcessReport$"},
		Dir:           dir,
		Env:           []string{"GOSHELL_RESUME_TEST_CHILD=1", resumeTokenEnv + "=" + token},
		NoExitMessage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tab := &Tab{name: "local", term: term}
	attachResume(tab, token, store)
	defer func() {
		detachResume(tab, false)
		term.Close()
	}()

	var records []*resumeRecord
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		records = store.list()
		if len(records) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(records) != 1 {
		t.Fatalf("the store holds %d records", len(records))
	}
	rec := records[0]
	if rec.PID < 1 {
		t.Errorf("the record is %+v", rec)
	}
	if len(rec.Argv) != 1 || rec.Argv[0] != "nvim" {
		t.Errorf("the command is %q", rec.Argv)
	}
	if !sameResumeDir(rec.Cwd, dir) {
		t.Errorf("the directory is %q, want %q", rec.Cwd, dir)
	}
}

// sameResumeDir says whether two paths name the same directory, as /var and
// /private/var do on macOS.
func sameResumeDir(a, b string) bool {
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

// resumeTestChild is the program of TestResumeProcessReport: it reports how
// to bring it back, as a program the terminal runs does, and stays alive
// while the terminal checks the report.
func resumeTestChild() {
	token := os.Getenv(resumeTokenEnv)
	if token == "" {
		// not the child the test started
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	body := "pid=" + strconv.Itoa(os.Getpid()) +
		":time=" + strconv.FormatInt(time.Now().Unix(), 10) +
		":argv=" + base64.StdEncoding.EncodeToString([]byte("nvim")) +
		":cwd=" + base64.StdEncoding.EncodeToString([]byte(cwd))
	os.Stdout.WriteString("\x1b]7577;" + resumeBody(body, token) + "\x1b\\")
	// the terminal checks that the program is running as it reads it
	time.Sleep(2 * time.Second)
}

// TestResumeCommandText checks the command of a record, as the user reads
// it: the characters that may mislead are taken out.
func TestResumeCommandText(t *testing.T) {
	rec := &resumeRecord{Argv: []string{"nvim", "-S", "/tmp/a b", "x‮y"}}
	got := resumeCommandText(rec)
	if strings.Contains(got, "‮") {
		t.Errorf("the command holds a right-to-left override: %q", got)
	}
	if !strings.Contains(got, "'/tmp/a b'") {
		t.Errorf("the command is %q", got)
	}
}

package main

// A manual check of the Program Resume Protocol with a real program: oni3
// reports how to bring itself back, the terminal goes away, and the record
// brings the same session back. It is not part of the suite, as it needs
// oni3 and its configuration:
//
//	GOSHELL_ONI3_TEST=1 go test -run TestManualOni3Resume -v .

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

func TestManualOni3Resume(t *testing.T) {
	if os.Getenv("GOSHELL_ONI3_TEST") != "1" {
		t.Skip("set GOSHELL_ONI3_TEST=1 to run")
	}
	dir := t.TempDir()
	work := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	store := openResumeStore(path)

	token := newResumeToken()
	term, err := terminal.New(terminal.Options{
		Command:       []string{"oni3"},
		Dir:           work,
		Env:           []string{resumeTokenEnv + "=" + token},
		NoExitMessage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tab := &Tab{name: "oni3", term: term}
	attachResume(tab, token, store)

	// a session is reported once it is saved, which a turn does: oni3's
	// interface takes the line as a message
	time.Sleep(2 * time.Second)
	term.Send([]byte("hello\r"))

	rec := waitOni3Record(store, 30*time.Second)
	if rec == nil {
		t.Fatalf("no report reached the store; the terminal shows:\n%s", tailText(term.Text(), 2000))
	}
	t.Logf("report: pid=%d argv=%q cwd=%q", rec.PID, rec.Argv, rec.Cwd)
	if len(rec.Argv) < 3 || filepath.Base(rec.Argv[0]) != "oni3" || rec.Argv[1] != "resume" {
		t.Fatalf("the command is %q", rec.Argv)
	}

	// the terminal goes away, as it does when the app restarts
	detachResume(tab, false)
	term.Close()
	store.flush()

	var resumable []*resumeRecord
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		resumable = openResumeStore(path).resumable()
		if len(resumable) == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(resumable) != 1 {
		t.Fatalf("resumable holds %d records", len(resumable))
	}
	old := resumable[0]
	t.Logf("resumable: %q in %q", old.Argv, old.Cwd)

	// the app starts the recorded command, as the dialog does
	store2 := openResumeStore(path)
	store2.remove(old.PID)
	store2.flush()
	token2 := newResumeToken()
	term2, err := terminal.New(terminal.Options{
		Command:       old.Argv,
		Dir:           old.Cwd,
		Env:           []string{resumeTokenEnv + "=" + token2},
		NoExitMessage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tab2 := &Tab{name: "oni3 resumed", term: term2}
	attachResume(tab2, token2, store2)
	defer func() {
		detachResume(tab2, true)
		term2.Close()
	}()

	rec2 := waitOni3Record(store2, 30*time.Second)
	if rec2 == nil {
		t.Fatalf("the resumed program did not report; the terminal shows:\n%s", tailText(term2.Text(), 2000))
	}
	t.Logf("resumed report: pid=%d argv=%q", rec2.PID, rec2.Argv)
	if len(rec2.Argv) != len(old.Argv) || rec2.Argv[2] != old.Argv[2] {
		t.Errorf("the resumed program reported %q, want the session of %q", rec2.Argv, old.Argv)
	}
}

// waitOni3Record waits for a report of a program to reach the store.
func waitOni3Record(store *resumeStore, timeout time.Duration) *resumeRecord {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		if records := store.list(); len(records) > 0 {
			return records[0]
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// tailText is the end of a terminal's text, for a failed check.
func tailText(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return text[len(text)-n:]
}

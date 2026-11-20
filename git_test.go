package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseGitStatus(t *testing.T) {
	out := "## main...origin/main [ahead 1, behind 2]\x00 M a.txt\x00A  b.txt\x00" +
		"RM c.txt\x00old.txt\x00?? d.txt\x00UU e.txt\x00"
	branch, ahead, behind, files := parseGitStatus(out)
	if branch != "main" || ahead != 1 || behind != 2 {
		t.Fatalf("branch %q ahead %d behind %d", branch, ahead, behind)
	}
	if len(files) != 5 {
		t.Fatalf("files: %+v", files)
	}
	if f := files[0]; f.path != "a.txt" || f.x != ' ' || f.y != 'M' {
		t.Fatalf("a modified file: %+v", f)
	}
	if f := files[1]; f.path != "b.txt" || f.x != 'A' || f.y != ' ' {
		t.Fatalf("a staged file: %+v", f)
	}
	if f := files[2]; f.path != "c.txt" || f.orig != "old.txt" || f.x != 'R' {
		t.Fatalf("a renamed file: %+v", f)
	}
	if f := files[4]; gitState(f) != "UU" {
		t.Fatalf("a conflict: %+v", f)
	}
	if got := gitState(files[3]); got != "U" {
		t.Fatalf("an untracked file says %q", got)
	}
}

func TestParseGitBranch(t *testing.T) {
	cases := []struct {
		in     string
		branch string
		ahead  int
		behind int
	}{
		{"main", "main", 0, 0},
		{"main...origin/main", "main", 0, 0},
		{"main...origin/main [ahead 3, behind 1]", "main", 3, 1},
		{"main...origin/main [behind 4]", "main", 0, 4},
		{"No commits yet on trunk", "trunk", 0, 0},
		{"HEAD (no branch)", "HEAD (no branch)", 0, 0},
	}
	for _, c := range cases {
		branch, ahead, behind := parseGitBranch(c.in)
		if branch != c.branch || ahead != c.ahead || behind != c.behind {
			t.Fatalf("parseGitBranch(%q) = %q, %d, %d", c.in, branch, ahead, behind)
		}
	}
}

func TestReadGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	src, ctx := localFiles{}, context.Background()
	git := func(args ...string) {
		t.Helper()
		if _, err := src.Git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "a.txt")
	git("-c", "user.email=a@b", "-c", "user.name=a", "commit", "-qm", "init")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := readGitStatus(ctx, src, dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.root == "" || st.branch == "" {
		t.Fatalf("status: %+v", st)
	}
	if len(st.files) != 2 {
		t.Fatalf("files: %+v", st.files)
	}

	// a folder outside a repository says so
	other := t.TempDir()
	if _, err := readGitStatus(ctx, src, other); err != errNoGitRepo {
		t.Fatalf("not a repository: %v", err)
	}
}

func TestParseGitLog(t *testing.T) {
	log := "h1\x1fp1 p2\x1fHEAD -> main, tag: v1\x1fMerge it\x1fAnn\x1f2 days ago\x1e" +
		"h2\x1f\x1forigin/main\x1finit\x1fBob\x1f3 days ago\x1e"
	commits := parseGitLog(log)
	if len(commits) != 2 {
		t.Fatalf("commits: %+v", commits)
	}
	if c := commits[0]; c.hash != "h1" || len(c.parents) != 2 || c.subject != "Merge it" ||
		len(c.refs) != 2 || c.refs[1] != "tag: v1" {
		t.Fatalf("the merge: %+v", c)
	}
	if c := commits[1]; len(c.parents) != 0 || c.refs[0] != "origin/main" {
		t.Fatalf("the first commit: %+v", c)
	}
}

func TestGitGraphRows(t *testing.T) {
	linear := gitGraphRows(parseGitLog(
		"c\x1fb\x1f\x1fthree\x1fA\x1fnow\x1e" +
			"b\x1fa\x1f\x1ftwo\x1fA\x1fnow\x1e" +
			"a\x1f\x1f\x1fone\x1fA\x1fnow\x1e"))
	if len(linear) != 3 {
		t.Fatalf("rows: %d", len(linear))
	}
	for i, row := range linear {
		if row.col != 0 || len(row.joins) != 0 {
			t.Fatalf("row %d: %+v", i, row)
		}
	}

	// a merge draws its second parent in another lane
	rows := gitGraphRows(parseGitLog(
		"m\x1fb d\x1f\x1fmerge\x1fA\x1fnow\x1e" +
			"b\x1fa\x1f\x1fmain work\x1fA\x1fnow\x1e" +
			"d\x1fa\x1f\x1fbranch work\x1fA\x1fnow\x1e" +
			"a\x1f\x1f\x1froot\x1fA\x1fnow\x1e"))
	if len(rows) != 4 {
		t.Fatalf("rows: %d", len(rows))
	}
	if rows[0].col != 0 || len(rows[0].joins) != 1 || rows[0].joins[0] != 1 {
		t.Fatalf("the merge row: %+v", rows[0])
	}
	if rows[2].col != 1 {
		t.Fatalf("the branch's commit is in lane %d", rows[2].col)
	}
	if rows[3].col != 0 || len(rows[3].in) == 0 || rows[3].in[0] != "a" || rows[3].in[1] != "a" {
		t.Fatalf("the root: col %d, lanes %v", rows[3].col, rows[3].in)
	}
	// the lane the root's commit already had joins it
	if len(rows[3].joins) != 1 || rows[3].joins[0] != 1 {
		t.Fatalf("the root's joins: %v", rows[3].joins)
	}
}

func TestNewDiffState(t *testing.T) {
	st := newDiffState("main.go", "package main\n", "package main\n\nfunc main() {}\n", false)
	if st.Old.String() != "package main\n" || st.New.String() != "package main\n\nfunc main() {}\n" {
		t.Fatalf("the texts: %q %q", st.Old.String(), st.New.String())
	}
	if st.Old.Language() == nil || st.New.Language() == nil {
		t.Fatal("no language")
	}
}

func TestParseCommitFiles(t *testing.T) {
	patch := `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,3 @@
 one
-two
+2
+three
diff --git a/new.go b/new.go
new file mode 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1 @@
+package main
diff --git a/old.txt b/renamed.txt
similarity index 100%
rename from old.txt
rename to renamed.txt
diff --git a/logo.png b/logo.png
index 1111111..2222222 100644
Binary files a/logo.png and b/logo.png differ
`
	files, more, err := parseCommitFiles(patch, false)
	if err != nil {
		t.Fatal(err)
	}
	if more != 0 || len(files) != 4 {
		t.Fatalf("files: %d, more: %d", len(files), more)
	}
	if f := files[0]; f.name != "a.txt" || f.added != 2 || f.removed != 1 || f.state == nil {
		t.Fatalf("a modified file: %+v", f)
	}
	if f := files[1]; f.name != "new.go" || f.added != 1 || f.removed != 0 {
		t.Fatalf("an added file: %+v", f)
	}
	if f := files[2]; f.name != "renamed.txt" || f.oldName != "old.txt" {
		t.Fatalf("a renamed file: %+v", f)
	}
	if f := files[3]; !f.binary || f.state != nil {
		t.Fatalf("a binary file: %+v", f)
	}
}

func TestCommitMeta(t *testing.T) {
	cd := &commitDiff{}
	cd.fillMeta("abc\x1fAnn\x1fa@example.com\x1f2026-01-02T03:04:05Z\x1f2 days ago\x1fHEAD -> main, tag: v1\x1fAdd a thing")
	if cd.author != "Ann" || cd.date == "" || cd.when != "2 days ago" || cd.subject != "Add a thing" {
		t.Fatalf("the commit: %+v", cd)
	}
	if len(cd.refs) != 2 || cd.refs[0] != "HEAD -> main" || cd.refs[1] != "tag: v1" {
		t.Fatalf("the refs: %v", cd.refs)
	}
}

func TestCommitChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	src, ctx := localFiles{}, context.Background()
	git := func(args ...string) {
		t.Helper()
		if _, err := src.Git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.email", "a@example.com")
	git("config", "user.name", "Ann")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := readGitStatus(ctx, src, dir)
	if err != nil {
		t.Fatal(err)
	}
	g := &gitView{dir: dir, status: st, message: "first commit"}
	g.syncSelection()

	w := newWindow()
	tab := &Tab{files: &fileBrowser{src: localFiles{}, path: dir}, git: g}
	w.commitChanges(tab)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if out, err := src.Git(ctx, dir, "log", "-1", "--format=%s"); err == nil && strings.TrimSpace(out) == "first commit" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the commit did not happen")
}

func TestSwitchBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	src, ctx := localFiles{}, context.Background()
	git := func(args ...string) {
		t.Helper()
		if _, err := src.Git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.email", "a@example.com")
	git("config", "user.name", "Ann")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644)
	git("add", "a.txt")
	git("-c", "user.email=a@example.com", "-c", "user.name=Ann", "commit", "-qm", "init")
	git("branch", "dev")

	w := newWindow()
	tab := &Tab{files: &fileBrowser{src: localFiles{}, path: dir}, git: &gitView{dir: dir}}
	w.switchBranch(tab, tab.git, "dev")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if out, err := src.Git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && strings.TrimSpace(out) == "dev" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the branch did not switch")
}

func TestGitHeadText(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	src, ctx := localFiles{}, context.Background()
	git := func(args ...string) {
		t.Helper()
		if _, err := src.Git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.email", "a@example.com")
	git("config", "user.name", "Ann")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "sub/a.txt")
	git("commit", "-qm", "init")
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &diffFile{
		src: src, gr: src, root: dir,
		path:    filepath.Join(dir, "sub", "a.txt"),
		relPath: "sub/a.txt",
	}
	old, err := gitHeadText(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if old != "one\n" {
		t.Fatalf("HEAD holds %q", old)
	}

	// a file HEAD does not have has nothing before it
	d = &diffFile{src: src, gr: src, root: dir, path: filepath.Join(dir, "new.txt"), relPath: "new.txt"}
	if old, err := gitHeadText(ctx, d); err != nil || old != "" {
		t.Fatalf("an added file: %q, %v", old, err)
	}
}

func TestGraphRefs(t *testing.T) {
	shown, more := graphRefs([]string{
		"origin/dependabot/go_mo/x", "HEAD -> main", "mygo", "tag: v0.3.0", "origin/main",
	})
	if more != 3 || len(shown) != 2 || shown[0] != "HEAD -> main" {
		t.Fatalf("shown %v, more %d", shown, more)
	}
	if got := refLabel("HEAD -> main"); got != "main" {
		t.Fatalf("HEAD -> main is %q", got)
	}
	if got := refLabel("tag: v1.0.0"); got != "v1.0.0" {
		t.Fatalf("a tag is %q", got)
	}
	if got := refLabel("origin/feature/x"); got != "feature/x" {
		t.Fatalf("a remote branch is %q", got)
	}
	if got := refLabel("release/1.0"); got != "release/1.0" {
		t.Fatalf("a local branch is %q", got)
	}
}

func TestCommitMatches(t *testing.T) {
	c := &gitCommit{hash: "abcdef1234", subject: "Fix the parser", author: "Ann Lee"}
	cases := map[string]bool{
		"":               true,
		"parser":         true,
		"PARSER":         true,
		"ann":            true,
		"abcdef":         true,
		"author:ann":     true,
		"author:bob":     false,
		"msg:fix":        true,
		"msg:parser ann": true,
		"hash:abc":       true,
		"hash:fff":       false,
		"fix ann":        true,
		"fix bob":        false,
	}
	for query, want := range cases {
		if got := commitMatches(c, query); got != want {
			t.Fatalf("commitMatches(%q) = %v", query, got)
		}
	}
}

func TestFilteredRows(t *testing.T) {
	commits := parseGitLog(
		"c\x1fb\x1f\x1fthree\x1fA\x1fnow\x1e" +
			"b\x1fa\x1f\x1ftwo\x1fB\x1fnow\x1e" +
			"a\x1f\x1f\x1fone\x1fA\x1fnow\x1e")
	kept := filterCommits(commits, "author:a")
	if len(kept) != 2 {
		t.Fatalf("kept %d", len(kept))
	}
	rows := gitGraphRowsKeeping(kept, commitSet(kept))
	if len(rows) != 2 {
		t.Fatalf("rows %d", len(rows))
	}
	// the newest commit's parent is not kept, so its lane ends at it
	if rows[0].out != nil && len(rows[0].out) > 0 && rows[0].out[0] == "b" {
		t.Fatalf("a lane waits for a commit the filter left out: %v", rows[0].out)
	}
	if graphCols(rows) != 1 {
		t.Fatalf("cols %d", graphCols(rows))
	}
}

func TestSyncSelection(t *testing.T) {
	g := &gitView{status: &gitStatus{files: []gitFile{{path: "a.txt"}, {path: "b.txt"}}}}
	g.syncSelection()
	if !g.selection["a.txt"] || !g.selection["b.txt"] {
		t.Fatalf("a new status: %v", g.selection)
	}
	g.selection["b.txt"] = false
	g.status.files = append(g.status.files, gitFile{path: "c.txt"})
	g.syncSelection()
	if g.selection["b.txt"] || !g.selection["c.txt"] {
		t.Fatalf("after a change: %v", g.selection)
	}
}

func TestCommitSelectedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	src, ctx := localFiles{}, context.Background()
	git := func(args ...string) string {
		t.Helper()
		out, err := src.Git(ctx, dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	git("init", "-q")
	git("config", "user.email", "a@example.com")
	git("config", "user.name", "Ann")
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("one\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-qm", "init")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := readGitStatus(ctx, src, dir)
	if err != nil {
		t.Fatal(err)
	}
	g := &gitView{dir: dir, status: st, message: "only a and c"}
	g.syncSelection()
	g.selection["b.txt"] = false

	w := newWindow()
	tab := &Tab{files: &fileBrowser{src: localFiles{}, path: dir}, git: g}
	w.commitChanges(tab)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, err := src.Git(ctx, dir, "log", "-1", "--name-only", "--format=%s")
		if err == nil && strings.Contains(out, "only a and c") {
			if strings.Contains(out, "b.txt") {
				t.Fatalf("the commit took b.txt: %q", out)
			}
			if !strings.Contains(out, "a.txt") || !strings.Contains(out, "c.txt") {
				t.Fatalf("the commit left a file out: %q", out)
			}
			status := git("status", "--porcelain")
			if !strings.Contains(status, "b.txt") {
				t.Fatalf("b.txt is no longer changed: %q", status)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the commit did not happen")
}

func TestStageFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	src, ctx := localFiles{}, context.Background()
	git := func(args ...string) {
		t.Helper()
		if _, err := src.Git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.email", "a@example.com")
	git("config", "user.name", "Ann")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := readGitStatus(ctx, src, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.files) != 1 || !isUntracked(st.files[0]) {
		t.Fatalf("the status: %+v", st.files)
	}

	w := newWindow()
	tab := &Tab{files: &fileBrowser{src: localFiles{}, path: dir}, git: &gitView{dir: dir, status: st}}
	w.stageFile(tab, src, st, st.files[0])

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, err := src.Git(ctx, dir, "status", "--porcelain")
		if err == nil && strings.HasPrefix(strings.TrimSpace(out), "A") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the file was not added")
}

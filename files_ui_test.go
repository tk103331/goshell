package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"goshell/locales"
)

type filesConn struct {
	io.Reader
	io.Writer
}

func (filesConn) Close() error { return nil }

// filesTester shows a window whose one session browses dir in the file
// sidebar.
func filesTester(t *testing.T, dir string) (*Window, *ui.Tester) {
	t.Helper()
	r, pw := io.Pipe()
	term, err := terminal.New(terminal.Options{
		Conn:      filesConn{Reader: r, Writer: pw},
		Theme:     termLight,
		DarkTheme: termDark,
	})
	if err != nil {
		t.Fatal(err)
	}
	go pw.Write([]byte("$ "))
	w := newWindow()
	w.appTheme = "light"
	w.tabs = []*Tab{{name: "local", term: term}}
	w.tabs[0].useFiles(localFiles{})
	w.selected = 0
	time.Sleep(50 * time.Millisecond)

	b := w.tabs[0].files
	b.path = dir
	b.started = true
	entries, err := b.src.List(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	b.entries = entries

	tester := ui.NewTester(w.view, 1200, 760)
	tester.Frame()
	return w, tester
}

func menuHas(menu []string, items ...string) bool {
	for _, item := range items {
		if !slices.Contains(menu, item) {
			return false
		}
	}
	return true
}

func TestFilesContextMenu(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, tester := filesTester(t, dir)

	if r, ok := tester.Find("src"); ok {
		tester.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
		menu := tester.Menu()
		if !menuHas(menu, locales.FilesOpen(), locales.FilesRename(), locales.FilesDelete(), locales.FilesCopyPath()) {
			t.Fatalf("a folder's menu: %v", menu)
		}
	}
	if r, ok := tester.Find("README.md"); ok {
		tester.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
		menu := tester.Menu()
		if !menuHas(menu, locales.FilesOpen(), locales.FilesRename(), locales.FilesDelete(), locales.FilesCopyName()) {
			t.Fatalf("a file's menu: %v", menu)
		}
	}

	// Copy Path puts the file's path on the clipboard
	if r, ok := tester.Find("README.md"); ok {
		tester.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
		if err := tester.ChooseMenuItem(locales.FilesCopyPath()); err != nil {
			t.Fatal(err)
		}
		if got, want := tester.Clipboard(), filepath.Join(dir, "README.md"); got != want {
			t.Fatalf("the clipboard says %q, want %q", got, want)
		}
	}
	// the + button in the header
	if err := tester.Click(locales.FilesNew()); err != nil {
		t.Fatal(err)
	}
	if menu := tester.Menu(); !menuHas(menu, locales.FilesNewFolder(), locales.FilesNewFile()) {
		t.Fatalf("the header's menu: %v", menu)
	}

	// the empty space under the rows
	tester.RightClickAt(1000, 500)
	menu := tester.Menu()
	if !menuHas(menu, locales.FilesNewFolder(), locales.FilesNewFile(), locales.FilesRefresh()) {
		t.Fatalf("the list's menu: %v", menu)
	}
}

func TestFilesNewPrompt(t *testing.T) {
	w, tester := filesTester(t, t.TempDir())
	w.filesNew(w.tabs[0], true)
	tester.Frame()
	if w.filePrompt == nil {
		t.Fatal("no dialog")
	}
	if !tester.HasText(locales.FilesNewFolderTitle()) {
		t.Fatalf("the dialog says %v", tester.Texts())
	}

	tester.Type("docs")
	tester.Frame()
	if w.filePrompt.value != "docs" {
		t.Fatalf("the name is %q", w.filePrompt.value)
	}

	// a name with a slash is refused, and the dialog stays
	w.filePrompt.value = "a/b"
	w.submitFilePrompt()
	tester.Frame()
	if w.filePrompt == nil || w.filePrompt.err != locales.FilesNameInvalid() {
		t.Fatalf("the dialog took a slash: %+v", w.filePrompt)
	}

	if r, ok := tester.Find(locales.BtnCancel()); ok {
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	tester.Frame()
	if w.filePrompt != nil {
		t.Fatalf("the dialog stayed open: %+v", w.filePrompt)
	}
}

func TestEditorView(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	f := &editorFile{path: filepath.Join(dir, "main.go"), name: "main.go"}
	if err := fillEditorFile(f, localFiles{}, f.name, []byte("package main\n\nfunc main() {}\n")); err != nil {
		t.Fatal(err)
	}
	w.paneTabs = []*paneTab{{ed: f}}
	w.paneSel = 0
	tester.Frame()
	if !tester.HasText("main.go") {
		t.Fatalf("the tabs say %v", tester.Texts())
	}
	if !tester.HasText("Code editor") {
		t.Fatalf("the editor shows %v", tester.Texts())
	}

	// a close with changes asks first, and cancel keeps the editor
	f.buf.Edit(editor.Edit{Start: 0, End: 0, Text: "// edited\n"})
	w.closePaneTab(0)
	tester.Frame()
	if w.confirm == nil {
		t.Fatal("no question")
	}
	if !tester.HasText(locales.EditorDiscardTitle()) {
		t.Fatalf("the question says %v", tester.Texts())
	}
	w.confirm = nil
	tester.Frame()
	if len(w.paneTabs) != 1 {
		t.Fatal("the editor closed with changes")
	}

	// Discard takes the tab away, and the terminal takes the room back
	w.closePaneTab(0)
	tester.Frame()
	if w.confirm == nil {
		t.Fatal("no question")
	}
	if r, ok := tester.Find(locales.BtnDiscard()); ok {
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	} else {
		t.Fatalf("the question has no Discard: %v", tester.Texts())
	}
	tester.Frame()
	if len(w.paneTabs) != 0 {
		t.Fatal("the tab stayed")
	}
	if tester.HasText("Code editor") {
		t.Fatal("the editor still shows")
	}
}

func TestPaneTabs(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	first := &editorFile{path: filepath.Join(dir, "a.go"), name: "a.go"}
	second := &editorFile{path: filepath.Join(dir, "b.go"), name: "b.go"}
	for _, f := range []*editorFile{first, second} {
		if err := fillEditorFile(f, localFiles{}, f.name, []byte("package main\n")); err != nil {
			t.Fatal(err)
		}
	}
	w.paneTabs = []*paneTab{{ed: first}, {ed: second}}
	w.paneSel = 0
	tester.Frame()

	// a tab's click selects it
	if err := tester.Click("b.go"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if w.paneSel != 1 {
		t.Fatalf("selected %d", w.paneSel)
	}

	// closing it closes its tab
	w.closePaneTab(w.paneSel)
	tester.Frame()
	if len(w.paneTabs) != 1 || w.selectedPane() != w.paneTabs[0] || w.paneTabs[0].ed != first {
		t.Fatalf("tabs after closing: %d", len(w.paneTabs))
	}
}

func TestPaneHandleDrag(t *testing.T) {
	w, tester := filesTester(t, t.TempDir())
	f := &editorFile{path: "/tmp/a.go", name: "a.go"}
	if err := fillEditorFile(f, localFiles{}, f.name, []byte("package main\n")); err != nil {
		t.Fatal(err)
	}
	w.paneTabs = []*paneTab{{ed: f}}
	w.paneSel = 0
	tester.Frame()

	// the handle is 8 DIPs between the terminal and the pane
	middle := float32(1200) - (w.sidebar + 8) - (w.filesWidth + 8)
	handle := 272 + (middle-8)*(1-w.paneShare)
	start := w.paneShare
	tester.Move(handle+4, 300)
	tester.Press(handle+4, 300)
	tester.Advance(10 * time.Millisecond)
	tester.Move(handle-56, 300)
	tester.Advance(10 * time.Millisecond)
	tester.Move(handle-60, 300)
	tester.Release(handle-60, 300)
	tester.Frame()
	if w.paneShare <= start {
		t.Fatalf("share %v after a drag from %v", w.paneShare, start)
	}
}

func TestGitView(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	st := &gitStatus{
		dir: dir, root: dir, branch: "main", ahead: 1, behind: 2,
		files: []gitFile{
			{path: "a.txt", x: ' ', y: 'M'},
			{path: "new.txt", x: 'A', y: ' '},
		},
	}
	w.tabs[0].git = &gitView{dir: dir, status: st}
	w.rightView = rightGit
	tester.Frame()
	if !tester.HasText("main") {
		t.Fatalf("the branch: %v", tester.Texts())
	}
	if !tester.HasText("a.txt") || !tester.HasText("new.txt") {
		t.Fatalf("the files: %v", tester.Texts())
	}
	if !tester.HasText("↑1") || !tester.HasText("↓2") {
		t.Fatalf("the upstream: %v", tester.Texts())
	}

	// the switcher goes back to the files
	if err := tester.Click(locales.FilesTitle()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if w.rightView != rightFiles {
		t.Fatalf("the view is %d", w.rightView)
	}
	if tester.HasText("↑1") {
		t.Fatalf("the git view stayed: %v", tester.Texts())
	}
}

func TestGitDiffTab(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	w.tabs[0].git = &gitView{dir: dir, status: &gitStatus{
		dir: dir, root: dir, branch: "main",
		files: []gitFile{{path: "a.txt", x: ' ', y: 'M'}},
	}}
	w.rightView = rightGit
	tester.Frame()

	// a double click on a changed file shows its diff in the pane
	if r, ok := tester.Find("a.txt"); ok {
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	} else {
		t.Fatalf("no file row: %v", tester.Texts())
	}
	tester.Frame()
	if len(w.paneTabs) != 1 || w.paneTabs[0].diff == nil {
		t.Fatalf("tabs: %+v", w.paneTabs)
	}
	if got := w.paneTabs[0].title(); got != "a.txt" {
		t.Fatalf("the tab is %q", got)
	}
}

func TestDiffTabShows(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	d := &diffFile{src: localFiles{}, path: filepath.Join(dir, "main.go"), name: "main.go"}
	d.state = newDiffState("main.go", "package main\n", "package main\n\nfunc main() {}\n", false)
	w.paneTabs = []*paneTab{{diff: d}}
	w.paneSel = 0
	tester.Frame()
	if !tester.HasText("main.go") {
		t.Fatalf("the tab: %v", tester.Texts())
	}
}

func TestGitGraphTab(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	commits := parseGitLog(
		"aaa\x1fbbb\x1fHEAD -> main\x1fsecond\x1fAnn\x1fnow\x1e" +
			"bbb\x1f\x1f\x1ffirst\x1fAnn\x1fnow\x1e")
	g := &graphTab{src: localFiles{}, dir: dir, commits: commits, rows: gitGraphRows(commits), cols: 1}
	if len(g.rows) == 0 {
		t.Fatal("no rows")
	}
	w.tabs[0].git = &gitView{dir: dir, status: &gitStatus{dir: dir, root: dir, branch: "main"}}
	w.paneTabs = []*paneTab{{graph: g}}
	w.paneSel = 0
	w.rightView = rightGit
	tester.Frame()
	if !tester.HasText("Git Graph") || !tester.HasText("second") || !tester.HasText("first") {
		t.Fatalf("the graph: %v", tester.Texts())
	}
}

func TestGitGraphButton(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	w.tabs[0].git = &gitView{dir: dir, status: &gitStatus{dir: dir, root: dir, branch: "main"}}
	w.rightView = rightGit
	tester.Frame()
	if err := tester.Click(locales.GitGraph()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if len(w.paneTabs) != 1 || w.paneTabs[0].graph == nil {
		t.Fatalf("tabs: %+v", w.paneTabs)
	}
}

func TestGitSplitView(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	commits := parseGitLog("aaa\x1f\x1f\x1ffirst\x1fAnn\x1fnow\x1e")
	g := &gitView{
		dir: dir,
		status: &gitStatus{dir: dir, root: dir, branch: "main",
			files: []gitFile{{path: "a.txt", x: ' ', y: 'M'}}},
		commits: commits,
		log:     gitGraphRows(commits),
		logCols: 1,
	}
	g.syncSelection()
	w.tabs[0].git = g
	w.rightView = rightGit
	tester.Frame()
	for _, want := range []string{locales.GitSelectNone(), locales.GitCommit(), "first", "main", "1/1"} {
		if !tester.HasText(want) {
			t.Fatalf("no %q in %v", want, tester.Texts())
		}
	}
}

func TestCommitTab(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	patch := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1,2 @@\n one\n+two\n"
	files, _, err := parseCommitFiles(patch, false)
	if err != nil {
		t.Fatal(err)
	}
	cd := &commitDiff{
		src: localFiles{}, dir: dir, hash: "abc1234", short: "abc1234",
		subject: "Add a thing", author: "Ann", when: "2 days ago",
		refs: []string{"HEAD -> main"}, files: files,
	}
	w.paneTabs = []*paneTab{{commit: cd}}
	w.paneSel = 0
	tester.Frame()
	for _, want := range []string{"Add a thing", "Ann", "2 days ago", "abc1234", "a.txt"} {
		if !tester.HasText(want) {
			t.Fatalf("no %q in %v", want, tester.Texts())
		}
	}
}

func TestGraphCommitOpens(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	commits := parseGitLog(
		"aaa\x1fbbb\x1fHEAD -> main\x1fsecond\x1fAnn\x1fnow\x1e" +
			"bbb\x1f\x1f\x1ffirst\x1fAnn\x1fnow\x1e")
	rows := gitGraphRows(commits)
	g := &graphTab{src: localFiles{}, gr: localFiles{}, dir: dir, commits: commits, rows: rows, cols: 1}
	w.paneTabs = []*paneTab{{graph: g}}
	w.paneSel = 0
	tester.Frame()
	if r, ok := tester.Find("second"); ok {
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	} else {
		t.Fatalf("no commit row: %v", tester.Texts())
	}
	tester.Frame()
	if len(w.paneTabs) != 2 || w.paneTabs[1].commit == nil {
		t.Fatalf("tabs: %+v", w.paneTabs)
	}
}

func TestCommitListSelection(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	commits := parseGitLog(
		"aaa\x1fbbb\x1fHEAD -> main\x1fsecond\x1fAnn\x1fnow\x1e" +
			"bbb\x1f\x1f\x1ffirst\x1fAnn\x1fnow\x1e")
	rows := gitGraphRows(commits)
	g := &graphTab{src: localFiles{}, gr: localFiles{}, dir: dir, commits: commits, rows: rows, cols: 1}
	w.paneTabs = []*paneTab{{graph: g}}
	w.paneSel = 0
	tester.Frame()

	if r, ok := tester.Find("second"); ok {
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	} else {
		t.Fatalf("no commit row: %v", tester.Texts())
	}
	tester.Frame()
	if g.selected != "aaa" {
		t.Fatalf("selected %q", g.selected)
	}

	// a double click also opens the commit
	if r, ok := tester.Find("first"); ok {
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	tester.Frame()
	if g.selected != "bbb" {
		t.Fatalf("selected %q", g.selected)
	}
	if len(w.paneTabs) != 2 || w.paneTabs[1].commit == nil {
		t.Fatalf("tabs: %+v", w.paneTabs)
	}
}

func TestGitBranchMenu(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	w.tabs[0].git = &gitView{
		dir:      dir,
		branches: []string{"dev", "trunk"},
		status:   &gitStatus{dir: dir, root: dir, branch: "trunk"},
	}
	w.rightView = rightGit
	tester.Frame()
	if err := tester.Click("trunk"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !tester.HasText("dev") || !tester.HasText(locales.GitSearchBranches()) {
		t.Fatalf("the branch popover: %v", tester.Texts())
	}
}

func TestGitSplitDrag(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	w.tabs[0].git = &gitView{
		dir:    dir,
		status: &gitStatus{dir: dir, root: dir, branch: "main", files: []gitFile{{path: "a.go", x: ' ', y: 'M'}}},
		log:    gitGraphRows(parseGitLog("aaa\x1f\x1fHEAD -> main\x1fone\x1fA\x1fnow\x1e")),
	}
	w.rightView = rightGit
	tester.Frame()
	start := w.gitShare

	// the splitter lies between the changes and the commits
	body := float32(760) - 10 - 10 - 36 - 32
	y := float32(10+36+32) + (body-8)*start + 4
	x := float32(1200 - 280 + 140)
	tester.Move(x, y)
	tester.Press(x, y)
	tester.Advance(10 * time.Millisecond)
	tester.Move(x, y+40)
	tester.Advance(10 * time.Millisecond)
	tester.Move(x, y+50)
	tester.Release(x, y+50)
	tester.Frame()
	if w.gitShare <= start {
		t.Fatalf("share %v after a drag from %v", w.gitShare, start)
	}
}

func TestMarkdownToggles(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	f := &editorFile{src: localFiles{}, path: filepath.Join(dir, "README.md"), name: "README.md", wrap: true}
	if err := fillEditorFile(f, localFiles{}, f.name, []byte("# Hi\n\nSome **text**.\n")); err != nil {
		t.Fatal(err)
	}
	f.md = newMarkdownOptions()
	w.paneTabs = []*paneTab{{ed: f}}
	w.paneSel = 0
	tester.Frame()

	// the rendered view first, and a click shows the source
	if err := tester.Click(locales.MarkdownSource()); err != nil {
		t.Fatalf("no source button: %v", tester.Texts())
	}
	tester.Frame()
	if !f.source {
		t.Fatal("the source did not show")
	}

	// the wrap button turns wrapping off
	if err := tester.Click(locales.EditorWrapOff()); err != nil {
		t.Fatalf("no wrap button: %v", tester.Texts())
	}
	tester.Frame()
	if f.wrap || f.state.SoftWrap {
		t.Fatalf("wrapping stayed on: %v %v", f.wrap, f.state.SoftWrap)
	}
}

func TestGraphFilter(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	commits := parseGitLog(
		"aaa\x1fbbb\x1fHEAD -> main\x1fsecond commit\x1fAnn\x1fnow\x1e" +
			"bbb\x1f\x1f\x1ffirst commit\x1fAnn\x1fnow\x1e")
	g := &graphTab{src: localFiles{}, gr: localFiles{}, dir: dir, commits: commits,
		rows: gitGraphRows(commits), cols: 1}
	w.paneTabs = []*paneTab{{graph: g}}
	w.paneSel = 0
	tester.Frame()
	if !tester.HasText("second commit") {
		t.Fatalf("the commits: %v", tester.Texts())
	}

	if err := tester.Click(locales.GitFilterCommits()); err != nil {
		t.Fatal(err)
	}
	tester.Type("first")
	tester.Frame()
	if tester.HasText("second commit") || !tester.HasText("first commit") {
		t.Fatalf("the filter shows %v", tester.Texts())
	}
}

func TestBranchSearch(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	w.tabs[0].git = &gitView{
		dir:      dir,
		branches: []string{"dev", "trunk"},
		status:   &gitStatus{dir: dir, root: dir, branch: "trunk"},
	}
	w.rightView = rightGit
	tester.Frame()
	if err := tester.Click("trunk"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	count := func(text string) int {
		n := 0
		for _, got := range tester.Texts() {
			if got == text {
				n++
			}
		}
		return n
	}
	baseDev, baseTrunk := count("dev"), count("trunk")
	if baseDev == 0 || baseTrunk == 0 {
		t.Fatalf("the branches: %v", tester.Texts())
	}

	if err := tester.Click(locales.GitSearchBranches()); err != nil {
		t.Fatal(err)
	}
	tester.Type("de")
	tester.Frame()
	if count("dev") != baseDev || count("trunk") != baseTrunk-1 {
		t.Fatalf("the filter shows %v", tester.Texts())
	}

	// a query that matches no branch leaves the list empty
	tester.Type("z")
	tester.Frame()
	if count("dev") != 0 {
		t.Fatalf("dev stayed: %v", tester.Texts())
	}
}

func TestGitSelection(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	g := &gitView{
		dir: dir,
		status: &gitStatus{dir: dir, root: dir, branch: "main", files: []gitFile{
			{path: "a.txt", x: ' ', y: 'M'},
			{path: "b.txt", x: ' ', y: 'M'},
		}},
	}
	g.syncSelection()
	w.tabs[0].git = g
	w.rightView = rightGit
	tester.Frame()
	if !tester.HasText("2/2") {
		t.Fatalf("the count: %v", tester.Texts())
	}

	// a click takes a file out of the commit
	if err := tester.Click("a.txt"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if g.selection["a.txt"] || !tester.HasText("1/2") {
		t.Fatalf("after a click: %v, selected %v", tester.Texts(), g.selection)
	}

	// select all takes it back
	if err := tester.Click(locales.GitSelectAll()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !g.selection["a.txt"] || !tester.HasText("2/2") {
		t.Fatalf("after selecting all: %v", g.selection)
	}
}

func TestGitAddUntracked(t *testing.T) {
	dir := t.TempDir()
	w, tester := filesTester(t, dir)
	g := &gitView{
		dir: dir,
		status: &gitStatus{dir: dir, root: dir, branch: "main", files: []gitFile{
			{path: "new.go", x: '?', y: '?'},
		}},
	}
	g.syncSelection()
	w.tabs[0].git = g
	w.rightView = rightGit
	tester.Frame()

	// the untracked file's row offers to add it, without taking it out of
	// the commit
	if err := tester.Click(locales.GitAddFile()); err != nil {
		t.Fatalf("no add button: %v", tester.Texts())
	}
	tester.Frame()
	if !g.selection["new.go"] {
		t.Fatalf("the add button took the file out: %v", g.selection)
	}
}

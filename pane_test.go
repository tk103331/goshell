package main

import (
	"testing"
)

func TestStripLineCol(t *testing.T) {
	cases := map[string]string{
		"/src/main.go":        "/src/main.go",
		"/src/main.go:12":     "/src/main.go",
		"/src/main.go:12:3":   "/src/main.go",
		"/src/a:12b":          "/src/a:12b",
		`C:\\src\\main.go`:    `C:\\src\\main.go`,
		`C:\\src\\main.go:12`: `C:\\src\\main.go`,
	}
	for in, want := range cases {
		if got := stripLineCol(in); got != want {
			t.Fatalf("stripLineCol(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWebTitle(t *testing.T) {
	if got := webTitle("https://www.example.com/a?b=1"); got != "example.com" {
		t.Fatalf("webTitle = %q", got)
	}
	if got := webTitle("https://docs.example.com/x"); got != "docs.example.com" {
		t.Fatalf("webTitle = %q", got)
	}
}

func TestPaneSelects(t *testing.T) {
	w := newWindow()
	a := &paneTab{ed: &editorFile{name: "a.go"}}
	b := &paneTab{ed: &editorFile{name: "b.go"}}
	w.paneTabs = []*paneTab{a, b}
	w.paneSel = 1
	if got := w.selectedPane(); got != b {
		t.Fatalf("selected %v", got)
	}
	w.removePaneTab(0)
	if len(w.paneTabs) != 1 || w.selectedPane() != b || w.paneSel != 0 {
		t.Fatalf("after removing: %d tabs, selected %d", len(w.paneTabs), w.paneSel)
	}
	w.removePaneTab(0)
	if w.selectedPane() != nil || w.paneSel != 0 {
		t.Fatalf("empty pane: selected %d", w.paneSel)
	}
}

func TestOpenEditorKeepsOneTab(t *testing.T) {
	w := newWindow()
	src := localFiles{}
	file := "/tmp/goshell-test/a.go"
	w.openEditor(src, file, "a.go")
	w.openEditor(src, file, "a.go")
	if len(w.paneTabs) != 1 {
		t.Fatalf("tabs: %d", len(w.paneTabs))
	}
	if w.selectedPane() == nil || w.selectedPane().ed == nil {
		t.Fatal("no editor selected")
	}
}

func TestOpenLink(t *testing.T) {
	w := newWindow()
	tab := &Tab{}

	if !w.openLink(tab, "https://www.example.com/a") {
		t.Fatal("a URL was left")
	}
	if len(w.paneTabs) != 1 || w.paneTabs[0].web == nil {
		t.Fatalf("tabs: %+v", w.paneTabs)
	}
	if got := w.paneTabs[0].title(); got != "example.com" {
		t.Fatalf("the tab is named %q", got)
	}
	// the same page shows the tab it has
	w.openLink(tab, "https://www.example.com/a")
	if len(w.paneTabs) != 1 {
		t.Fatalf("a second tab: %d", len(w.paneTabs))
	}

	// a file path a command printed opens in the editor
	tab = &Tab{files: &fileBrowser{src: localFiles{}, path: "/tmp"}}
	if !w.openLink(tab, "/tmp/x.go:12:3") {
		t.Fatal("a path was left")
	}
	if len(w.paneTabs) != 2 || w.paneTabs[1].ed == nil {
		t.Fatalf("tabs: %+v", w.paneTabs)
	}
	if got := w.paneTabs[1].ed; got.name != "x.go" || got.path != "/tmp/x.go" {
		t.Fatalf("the editor: %q %q", got.name, got.path)
	}

	// a scheme that is not the web goes to the system
	if w.openLink(tab, "mailto:a@example.com") {
		t.Fatal("mailto was taken")
	}
}

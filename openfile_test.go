package main

import (
	"bytes"
	"context"
	"io"
	"path"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// browseOnly is a file source that only reads, for the tests.
type browseOnly struct{}

func (browseOnly) Home(context.Context) (string, error)              { return "/", nil }
func (browseOnly) List(context.Context, string) ([]fileEntry, error) { return nil, nil }
func (browseOnly) Join(dir, name string) string                      { return path.Join(dir, name) }
func (browseOnly) Parent(dir string) string                          { return path.Dir(dir) }
func (browseOnly) Close() error                                      { return nil }

func TestIsBinary(t *testing.T) {
	if isBinary([]byte("package main\n")) {
		t.Fatal("text is binary")
	}
	if !isBinary([]byte("GIF89a\x00\x01")) {
		t.Fatal("a NUL byte is text")
	}
}

func TestReadLimited(t *testing.T) {
	data, err := readLimited(strings.NewReader("hello"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("read %q, %v", data, err)
	}
	big := io.LimitReader(zeroReader{}, openFileMax+1)
	if _, err := readLimited(big); err == nil {
		t.Fatal("a file too large was read")
	}
}

func TestFillEditorFile(t *testing.T) {
	f := &editorFile{}
	if err := fillEditorFile(f, localFiles{}, "main.go", []byte("package main\n")); err != nil {
		t.Fatal(err)
	}
	if f.readOnly || f.state == nil || f.state.ReadOnly {
		t.Fatalf("a writable file: %+v", f)
	}
	if f.buf.String() != "package main\n" {
		t.Fatalf("the text is %q", f.buf.String())
	}
	if f.buf.Language() != editor.LanguageFor("main.go") {
		t.Fatalf("the language is %v", f.buf.Language())
	}

	if err := fillEditorFile(&editorFile{}, localFiles{}, "logo.png", []byte("\x89PNG\x00")); err == nil {
		t.Fatal("a binary file opened")
	}

	r := &editorFile{}
	if err := fillEditorFile(r, browseOnly{}, "notes.txt", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if !r.readOnly || !r.state.ReadOnly {
		t.Fatalf("a browse-only file: %+v", r)
	}
}

func TestEditorFileModified(t *testing.T) {
	f := &editorFile{}
	if err := fillEditorFile(f, localFiles{}, "a.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if f.modified() {
		t.Fatal("a new file is modified")
	}
	f.buf.Edit(editor.Edit{Start: 0, End: 0, Text: "// x\n"})
	if !f.modified() {
		t.Fatal("an edited file is not modified")
	}
	if !bytes.HasPrefix([]byte(f.buf.String()), []byte("// x\n")) {
		t.Fatalf("the text is %q", f.buf.String())
	}
}

func TestKindOf(t *testing.T) {
	cases := map[string]fileKind{
		"a.go":         kindText,
		"Makefile":     kindText,
		"README.md":    kindMarkdown,
		"doc.markdown": kindMarkdown,
		"logo.png":     kindImage,
		"photo.JPG":    kindImage,
		"icon.svg":     kindImage,
		"anim.gif":     kindImage,
		"archive.tar":  kindText,
	}
	for name, want := range cases {
		if got := kindOf(name); got != want {
			t.Fatalf("kindOf(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestOpenFileTab(t *testing.T) {
	w := newWindow()
	w.openFileTab(localFiles{}, "/tmp/logo.png", "logo.png")
	if len(w.paneTabs) != 1 || w.paneTabs[0].image == nil {
		t.Fatalf("a picture: %+v", w.paneTabs)
	}
	w.openFileTab(localFiles{}, "/tmp/README.md", "README.md")
	if len(w.paneTabs) != 2 || w.paneTabs[1].ed == nil || w.paneTabs[1].ed.md == nil {
		t.Fatalf("Markdown: %+v", w.paneTabs)
	}
	w.openFileTab(localFiles{}, "/tmp/a.go", "a.go")
	if len(w.paneTabs) != 3 || w.paneTabs[2].ed == nil || w.paneTabs[2].ed.md != nil {
		t.Fatalf("a text file: %+v", w.paneTabs)
	}
}

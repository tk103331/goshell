package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"goshell/locales"
)

// fileKind is what a file opens as.
type fileKind int

const (
	kindText fileKind = iota
	kindMarkdown
	kindImage
)

// kindOf says what a file opens as, by its name.
func kindOf(name string) fileKind {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return kindMarkdown
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return kindImage
	}
	return kindText
}

// openFileTab opens a file as what it is: a picture in its preview, a
// Markdown file rendered, or any other text in the editor.
func (w *Window) openFileTab(src fileSource, file, name string) {
	if kindOf(name) == kindImage {
		w.openImage(src, file, name)
		return
	}
	w.openEditor(src, file, name)
}

// editorFile is a file the pane opened in the editor.
type editorFile struct {
	src      fileSource
	path     string // where the file is in its source
	name     string // the name the tab and the editor show
	buf      *editor.Buffer
	state    *editor.State
	loading  bool
	saving   bool
	readOnly bool
	focused  bool // the editor had the keyboard as of the last frame
	err      string

	// a Markdown file: its view's options, and whether it shows its source
	md     *markdown.Options
	source bool
	// wrap wraps the editor's lines
	wrap bool
}

func (f *editorFile) modified() bool { return f.buf != nil && f.buf.Modified() }

// newMarkdownOptions is the view options of a Markdown file.
func newMarkdownOptions() *markdown.Options { return &markdown.Options{} }

// isBinary reports whether data looks like a file that is not text: one
// with a NUL byte near its start.
func isBinary(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return bytes.IndexByte(data, 0) >= 0
}

// readFile reads what a file of any source holds, as the editor opens it.
func readFile(ctx context.Context, src fileSource, file string) ([]byte, error) {
	if local, ok := src.(fileLocal); ok {
		if p, ok := local.LocalPath(file); ok {
			f, err := os.Open(p)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			return readLimited(f)
		}
	}
	rd, ok := src.(fileReader)
	if !ok {
		return nil, errors.New(locales.FilesNotText())
	}
	rc, err := rd.Reader(ctx, file)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readLimited(rc)
}

// readLimited reads what r holds, up to the most the editor opens.
func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, openFileMax+1))
	if err != nil {
		return nil, err
	}
	if len(data) > openFileMax {
		return nil, errors.New(locales.FilesTooLarge())
	}
	return data, nil
}

// fillEditorFile gives the editor what a file's text makes of it.
func fillEditorFile(f *editorFile, src fileSource, name string, data []byte) error {
	if isBinary(data) {
		return errors.New(locales.FilesNotText())
	}
	f.buf = editor.NewBuffer(strings.ToValidUTF8(string(data), "\uFFFD"))
	f.buf.SetLanguage(editor.LanguageFor(name))
	_, writable := src.(fileWriter)
	f.readOnly = !writable
	f.state = &editor.State{
		Buffer:         f.buf,
		SoftWrap:       true,
		KeepScrollbars: true,
		ReadOnly:       f.readOnly,
	}
	return nil
}

// loadEditorFile reads a file into its editor, in the background.
func (w *Window) loadEditorFile(f *editorFile) {
	f.loading, f.err = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		data, err := readFile(ctx, f.src, f.path)
		w.onMain(func() {
			if !w.hasEditor(f) {
				return
			}
			f.loading = false
			if err != nil {
				f.err = err.Error()
				return
			}
			if err := fillEditorFile(f, f.src, f.name, data); err != nil {
				f.err = err.Error()
				return
			}
			w.syncMenus()
		})
	}()
}

// hasEditor reports whether the pane still holds the editor.
func (w *Window) hasEditor(f *editorFile) bool {
	for _, t := range w.paneTabs {
		if t.ed == f {
			return true
		}
	}
	return false
}

// saveEditor writes the selected editor's text back to its file.
func (w *Window) saveEditor() {
	tab := w.selectedPane()
	if tab == nil || tab.ed == nil {
		return
	}
	f := tab.ed
	if f.buf == nil || f.saving || f.readOnly {
		return
	}
	wr, ok := f.src.(fileWriter)
	if !ok {
		return
	}
	text, file := f.buf.String(), f.path
	f.saving = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		err := wr.Write(ctx, file, strings.NewReader(text))
		w.onMain(func() {
			f.saving = false
			if err != nil {
				w.errMsg, w.errOpen = err.Error(), true
				return
			}
			if f.buf.String() == text {
				f.buf.MarkSaved()
			}
			w.showToast(locales.EditorSaved(), "success")
			w.invalidate()
		})
	}()
}

// editorBody is the editor of the pane's selected tab, below its tabs: a
// Markdown file rendered unless it shows its source.
func (w *Window) editorBody(c *ui.Context, lk *look, f *editorFile) {
	switch {
	case f.loading:
		ui.Column(c).FillWidth().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
	case f.err != "":
		ui.Column(c).FillWidth().Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, f.err).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.35).MaxWidth(320)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				w.loadEditorFile(f)
			}
		})
	case f.md != nil && !f.source:
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(14).Children(func() {
				markdown.View(c, f.buf.String(), f.md)
			})
		})
	default:
		view := editor.View(c, f.state).FillWidth().Grow(1).AutoFocus()
		f.focused = view.FocusWithin()
	}
}

// editorFind opens the find bar of the selected editor, when it has the
// keyboard, and reports whether one took the key.
func (w *Window) editorFind() bool {
	tab := w.selectedPane()
	if tab == nil || tab.ed == nil || !tab.ed.focused || tab.ed.state == nil {
		return false
	}
	tab.ed.state.Run(editor.Find)
	return true
}

// editorMarkdownButton shows a Markdown file rendered, or its source.
func (w *Window) editorMarkdownButton(c *ui.Context, lk *look, f *editorFile) {
	tip, icon := locales.MarkdownSource(), iconEdit
	if f.source {
		tip, icon = locales.MarkdownRendered(), iconDocument
	}
	b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(tip)
	if b.Hovered() {
		b.Background(lk.chip)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(12, 12).TextColor(lk.textMuted)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		f.source = !f.source
	}
}

// editorWrapButton turns the editor's wrapping of its lines on and off.
func (w *Window) editorWrapButton(c *ui.Context, lk *look, f *editorFile) {
	tip := locales.EditorWrapOff()
	fg := lk.accent
	if !f.wrap {
		tip, fg = locales.EditorWrapOn(), lk.textMuted
	}
	b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(tip)
	if b.Hovered() {
		b.Background(lk.chip)
	}
	b.Children(func() {
		ui.Icon(c, iconWrap).Size(12, 12).TextColor(fg)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		f.wrap = !f.wrap
		if f.state != nil {
			f.state.SoftWrap = f.wrap
		}
	}
}

// editorExternalButton opens the editor's file with the app the system
// has for it.
func (w *Window) editorExternalButton(c *ui.Context, lk *look, f *editorFile) {
	b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(locales.EditorOpenExternal())
	if b.Hovered() {
		b.Background(lk.chip)
	}
	if f.loading || f.err != "" {
		b.Disabled(true)
	}
	b.Children(func() {
		ui.Icon(c, iconExternal).Size(12, 12).TextColor(lk.textMuted)
	})
	b.Tooltip(locales.EditorOpenExternal())
	if b.Clicked() {
		w.openExternally(f.src, f.path)
	}
}

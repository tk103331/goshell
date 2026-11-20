package main

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"goshell/locales"
)

// diffFile is a file's changes in the pane: what HEAD holds, what the file
// holds now, and the git that said so.
type diffFile struct {
	src     fileSource
	gr      gitRunner
	root    string // the repository's root
	path    string // the file's path in the source
	relPath string // the file in the repository, before a rename, for HEAD
	name    string
	deleted bool
	state   *editor.DiffState
	loading bool
	err     string
}

// openDiff shows a file's changes in a tab of the pane, or the tab it has.
func (w *Window) openDiff(src fileSource, gr gitRunner, root string, f gitFile) {
	file := src.Join(root, f.path)
	rel := f.path
	if f.orig != "" {
		rel = f.orig
	}
	if d := w.paneDiff(src, file); d != nil {
		for i, tab := range w.paneTabs {
			if tab.diff == d {
				w.paneSel = i
				break
			}
		}
		w.syncMenus()
		w.invalidate()
		return
	}
	d := &diffFile{
		src:     src,
		gr:      gr,
		root:    root,
		path:    file,
		relPath: rel,
		name:    pathBase(f.path),
		deleted: f.x == 'D' || f.y == 'D',
		loading: true,
	}
	w.paneTabs = append(w.paneTabs, &paneTab{diff: d})
	w.paneSel = len(w.paneTabs) - 1
	w.syncMenus()
	w.loadDiff(d)
}

// paneDiff is the diff of a file the pane holds, or nil.
func (w *Window) paneDiff(src fileSource, file string) *diffFile {
	for _, t := range w.paneTabs {
		if t.diff != nil && t.diff.src == src && t.diff.path == file {
			return t.diff
		}
	}
	return nil
}

// hasDiff reports whether the pane still holds the diff.
func (w *Window) hasDiff(d *diffFile) bool {
	for _, t := range w.paneTabs {
		if t.diff == d {
			return true
		}
	}
	return false
}

// loadDiff reads what HEAD holds of the file and what it holds now, in the
// background.
func (w *Window) loadDiff(d *diffFile) {
	d.loading, d.err = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		oldText, err := gitHeadText(ctx, d)
		if err != nil {
			w.diffFailed(d, err)
			return
		}
		newText := ""
		if !d.deleted {
			data, err := readFile(ctx, d.src, d.path)
			if err != nil {
				w.diffFailed(d, err)
				return
			}
			if isBinary(data) {
				w.diffFailed(d, errors.New(locales.FilesNotText()))
				return
			}
			newText = string(data)
		}
		w.onMain(func() {
			if !w.hasDiff(d) {
				return
			}
			d.loading = false
			d.state = newDiffState(d.name, oldText, newText, w.diffSplit)
			w.invalidate()
		})
	}()
}

// gitHeadText is what HEAD holds of a file, "" for one it does not have.
func gitHeadText(ctx context.Context, d *diffFile) (string, error) {
	// git names a file of HEAD by its path in the repository, not here
	out, err := d.gr.Git(ctx, d.root, "show", "HEAD:"+d.relPath)
	if err != nil {
		// a file added since HEAD, or untracked, has nothing before it
		return "", nil
	}
	if len(out) > openFileMax {
		return "", errors.New(locales.FilesTooLarge())
	}
	if isBinary([]byte(out)) {
		return "", errors.New(locales.FilesNotText())
	}
	return out, nil
}

// diffFailed shows what went wrong with a diff, in its tab.
func (w *Window) diffFailed(d *diffFile, err error) {
	w.onMain(func() {
		if !w.hasDiff(d) {
			return
		}
		d.loading = false
		d.err = err.Error()
		w.invalidate()
	})
}

// newDiffState compares a file's text before and after, in its language,
// in one column or side by side.
func newDiffState(name, oldText, newText string, split bool) *editor.DiffState {
	layout := editor.DiffUnified
	if split {
		layout = editor.DiffSplit
	}
	lang := editor.LanguageFor(name)
	before := editor.NewBuffer(strings.ToValidUTF8(oldText, "\uFFFD"))
	after := editor.NewBuffer(strings.ToValidUTF8(newText, "\uFFFD"))
	before.SetLanguage(lang)
	after.SetLanguage(lang)
	return &editor.DiffState{
		Old:            before,
		New:            after,
		Layout:         layout,
		Wrap:           !split,
		ContextLines:   3,
		KeepScrollbars: true,
	}
}

// diffBody is the pane's diff: a file's changes, one over the other.
func (w *Window) diffBody(c *ui.Context, lk *look, d *diffFile) {
	switch {
	case d.loading || d.state == nil && d.err == "":
		ui.Column(c).Fill().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
	case d.err != "":
		ui.Column(c).Fill().Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, d.err).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.35).MaxWidth(320)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				w.loadDiff(d)
			}
		})
	default:
		editor.DiffView(c, d.state).Fill().Grow(1)
	}
}

// diffButtons are the diff's own actions: the layout, and reading it again.
func (w *Window) diffButtons(c *ui.Context, lk *look, d *diffFile) {
	if d.state != nil {
		w.diffSplitButton(c, lk)
	}
	w.rowButton(c, lk, iconRefresh, locales.FilesRefresh(), func() { w.loadDiff(d) })
}

// diffSplitButton switches between one column and side by side, for every
// diff the pane holds.
func (w *Window) diffSplitButton(c *ui.Context, lk *look) {
	tip := locales.DiffSideBySide()
	if w.diffSplit {
		tip = locales.DiffInline()
	}
	b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(tip)
	if b.Hovered() {
		b.Background(lk.chip)
	}
	b.Children(func() {
		ui.Icon(c, iconColumns).Size(12, 12).TextColor(lk.textMuted)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		w.setDiffSplit(!w.diffSplit)
	}
}

// setDiffSplit shows the diffs in one column or side by side.
func (w *Window) setDiffSplit(split bool) {
	w.diffSplit = split
	layout := editor.DiffUnified
	if split {
		layout = editor.DiffSplit
	}
	for _, t := range w.paneTabs {
		if t.diff != nil && t.diff.state != nil {
			t.diff.state.Layout = layout
			t.diff.state.Wrap = !split
		}
		if t.commit != nil {
			for _, f := range t.commit.files {
				if f.state != nil {
					f.state.Layout = layout
					f.state.Wrap = !split
				}
			}
		}
	}
}

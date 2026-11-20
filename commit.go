package main

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"goshell/locales"
)

// commitDiff is a commit in the pane: what it says, and the files it
// changed.
type commitDiff struct {
	src     fileSource
	gr      gitRunner
	dir     string
	hash    string
	short   string
	subject string
	author  string
	date    string
	when    string
	refs    []string
	files   []commitFile
	more    int // files the diff leaves out
	loading bool
	err     string
}

// commitFile is one file a commit changed.
type commitFile struct {
	name    string
	oldName string
	binary  bool
	added   int
	removed int
	folded  bool // its diff is put away
	state   *editor.DiffState
}

// commitFileMax is the most files of a commit the pane shows a diff of.
const commitFileMax = 50

// openCommit shows a commit in a tab of the pane, or the tab it has.
func (w *Window) openCommit(src fileSource, gr gitRunner, dir, hash string) {
	if cd := w.paneCommit(src, hash); cd != nil {
		for i, tab := range w.paneTabs {
			if tab.commit == cd {
				w.paneSel = i
				break
			}
		}
		w.syncMenus()
		w.invalidate()
		return
	}
	cd := &commitDiff{src: src, gr: gr, dir: dir, hash: hash, short: shortHash(hash), loading: true}
	w.paneTabs = append(w.paneTabs, &paneTab{commit: cd})
	w.paneSel = len(w.paneTabs) - 1
	w.syncMenus()
	w.loadCommit(cd)
}

// paneCommit is the commit the pane holds, or nil.
func (w *Window) paneCommit(src fileSource, hash string) *commitDiff {
	for _, t := range w.paneTabs {
		if t.commit != nil && t.commit.src == src && t.commit.hash == hash {
			return t.commit
		}
	}
	return nil
}

// holdsCommit reports whether the pane still holds the commit.
func (w *Window) holdsCommit(cd *commitDiff) bool {
	for _, t := range w.paneTabs {
		if t.commit == cd {
			return true
		}
	}
	return false
}

// shortHash is a commit's hash as git shows it.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// loadCommit reads what a commit says and the patch of its files, in the
// background.
func (w *Window) loadCommit(cd *commitDiff) {
	cd.loading, cd.err = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		meta, err := cd.gr.Git(ctx, cd.dir, "show", "-s",
			"--format=%H%x1f%an%x1f%ae%x1f%aI%x1f%ar%x1f%D%x1f%s", cd.hash)
		if err != nil {
			w.commitLoadFailed(cd, err)
			return
		}
		patch, err := cd.gr.Git(ctx, cd.dir, "show", "--format=", "--patch", "--no-color", cd.hash)
		if err != nil {
			w.commitLoadFailed(cd, err)
			return
		}
		if len(patch) > openFileMax {
			w.commitLoadFailed(cd, errors.New(locales.FilesTooLarge()))
			return
		}
		parsed, more, err := parseCommitFiles(patch, w.diffSplit)
		if err != nil {
			w.commitLoadFailed(cd, err)
			return
		}
		w.onMain(func() {
			if !w.holdsCommit(cd) {
				return
			}
			cd.loading = false
			cd.fillMeta(meta)
			cd.files, cd.more = parsed, more
			w.invalidate()
		})
	}()
}

// parseCommitFiles makes the diffs of a commit's patch: the files it
// changed, the most commitFileMax of them, and how many it left out.
func parseCommitFiles(patch string, split bool) ([]commitFile, int, error) {
	files, err := editor.ParsePatch(patch)
	if err != nil {
		return nil, 0, err
	}
	more := 0
	if len(files) > commitFileMax {
		more = len(files) - commitFileMax
		files = files[:commitFileMax]
	}
	parsed := make([]commitFile, 0, len(files))
	for _, p := range files {
		f := commitFile{name: p.NewName, oldName: p.OldName, binary: p.Binary}
		if f.name == "" {
			f.name = p.OldName
		}
		if f.oldName == f.name {
			f.oldName = ""
		}
		if !p.Binary {
			layout := editor.DiffUnified
			if split {
				layout = editor.DiffSplit
			}
			st := &editor.DiffState{FitContent: true, Layout: layout, Wrap: !split, ContextLines: 3}
			st.SetPatch(p)
			f.added, f.removed = st.Stats()
			f.state = st
		}
		parsed = append(parsed, f)
	}
	return parsed, more, nil
}

// fillMeta reads what git show says of a commit.
func (cd *commitDiff) fillMeta(meta string) {
	fields := strings.SplitN(strings.TrimSpace(meta), "\x1f", 7)
	if len(fields) < 7 {
		return
	}
	cd.author, cd.date, cd.when = fields[1], fields[3], fields[4]
	cd.subject = fields[6]
	for _, ref := range strings.Split(fields[5], ", ") {
		if ref = strings.TrimSpace(ref); ref != "" {
			cd.refs = append(cd.refs, ref)
		}
	}
}

// commitLoadFailed shows what went wrong with a commit, in its tab.
func (w *Window) commitLoadFailed(cd *commitDiff, err error) {
	w.onMain(func() {
		if !w.holdsCommit(cd) {
			return
		}
		cd.loading = false
		cd.err = err.Error()
		w.invalidate()
	})
}

// commitBody is the pane's commit: what it says, and its files' diffs.
func (w *Window) commitBody(c *ui.Context, lk *look, cd *commitDiff) {
	switch {
	case cd.loading:
		ui.Column(c).Fill().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
	case cd.err != "":
		ui.Column(c).Fill().Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, cd.err).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.35).MaxWidth(320)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				w.loadCommit(cd)
			}
		})
	default:
		ui.Scroll(c).Fill().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				w.commitHeader(c, lk, cd)
				if len(cd.files) == 0 {
					ui.Column(c).FillWidth().Padding(16).Children(func() {
						ui.Text(c, locales.GitNoFileChanges()).FontSize(12).TextColor(lk.textMuted)
					})
					return
				}
				for i := range cd.files {
					w.commitFileView(c, lk, &cd.files[i])
				}
				if cd.more > 0 {
					ui.Text(c, locales.GitMoreFiles(cd.more)).FontSize(11.5).
						TextColor(lk.textFaint).Padding(10, 12)
				}
				ui.Box(c).Height(12)
			})
		})
	}
}

// commitHeader is what a commit says, above its files.
func (w *Window) commitHeader(c *ui.Context, lk *look, cd *commitDiff) {
	added, removed := 0, 0
	for _, f := range cd.files {
		added, removed = added+f.added, removed+f.removed
	}
	ui.Column(c).FillWidth().Padding(12).Gap(6).Shrink(0).
		BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
		ui.Text(c, cd.subject).FontSize(13.5).FontWeight(650).TextColor(lk.text).
			LineHeight(1.35)
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, cd.author).FontSize(11.5).TextColor(lk.textMuted).SingleLine().
				Tooltip(cd.date)
			ui.Text(c, "·").FontSize(11.5).TextColor(lk.textFaint)
			ui.Text(c, cd.when).FontSize(11.5).TextColor(lk.textMuted).SingleLine()
			ui.Text(c, cd.short).FontSize(10.5).TextColor(lk.textFaint).Padding(1, 5).
				Radius(4).Background(lk.chip).SingleLine()
			for _, ref := range cd.refs {
				w.gitRefChip(c, lk, ref)
			}
		})
		if len(cd.files) > 0 {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.GitFilesChanged(len(cd.files)+cd.more)).FontSize(11).
					TextColor(lk.textMuted).SingleLine()
				ui.Textf(c, "+%d", added).FontSize(11).TextColor(lk.success)
				ui.Textf(c, "−%d", removed).FontSize(11).TextColor(lk.danger)
			})
		}
	})
}

// commitFileView is one file a commit changed: its name, a click that puts
// its diff away or shows it again, and the diff.
func (w *Window) commitFileView(c *ui.Context, lk *look, f *commitFile) {
	row := ui.Row(c).FillWidth().Height(32).Padding(8, 12).Gap(6).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(lk.divider)
	if row.Hovered() {
		row.Background(lk.rowHover)
		row.Cursor(ui.CursorPointer)
	}
	row.Tooltip(f.name)
	row.Children(func() {
		chevron := iconChevronDown
		if f.folded {
			chevron = iconChevronRight
		}
		ui.Icon(c, chevron).Size(11, 11).TextColor(lk.textFaint)
		ui.Icon(c, iconFile).Size(12, 12).TextColor(lk.textFaint)
		ui.Text(c, f.name).FontSize(12).FontWeight(600).TextColor(lk.text).Grow(1).
			SingleLine().Ellipsis("…")
		if f.oldName != "" {
			ui.Text(c, f.oldName).FontSize(11).TextColor(lk.textFaint).SingleLine().
				Ellipsis("…").MaxWidth(120)
		}
		if f.binary {
			ui.Text(c, locales.GitBinaryFile()).FontSize(11).TextColor(lk.textFaint).SingleLine()
		} else {
			ui.Textf(c, "+%d", f.added).FontSize(10.5).TextColor(lk.success)
			ui.Textf(c, "−%d", f.removed).FontSize(10.5).TextColor(lk.danger)
		}
	})
	if row.Clicked() {
		f.folded = !f.folded
	}
	if !f.folded && f.state != nil {
		editor.DiffView(c, f.state).FillWidth()
	}
}

// commitButtons are the commit's own actions: the layout of its diffs.
func (w *Window) commitButtons(c *ui.Context, lk *look, cd *commitDiff) {
	w.diffSplitButton(c, lk)
}

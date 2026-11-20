package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// errNoGitRepo says a folder is not in a Git repository.
var errNoGitRepo = errors.New("not a git repository")

// gitStatus is what the sidebar's Git view shows of a folder: its branch,
// how far it is from its upstream, and the files that changed.
type gitStatus struct {
	dir    string // the folder the status was asked for
	root   string // the repository's root
	branch string
	ahead  int
	behind int
	files  []gitFile
}

// gitFile is one changed file of a repository.
type gitFile struct {
	path string // in the repository's root
	orig string // where a rename or a copy came from, ""
	x, y byte   // the index's and the working tree's state
}

// gitView is the Git view of one session: the folder it shows, its status,
// the commits of its branch, and the commit being written.
type gitView struct {
	dir     string
	status  *gitStatus
	loading bool
	err     string
	request uint64

	log        []gitGraphRow
	logCols    int
	commits    []*gitCommit
	filter     commitFilter
	logLoading bool
	logErr     string
	logRequest uint64

	branchOpen  bool
	branchQuery string
	selection   map[string]bool // the changed files the commit takes

	branches  []string
	switching bool
	selected  string // the commit the list shows as chosen

	message    string
	committing bool
	commitErr  string
}

// readGitStatus asks a source's git for a folder's status.
func readGitStatus(ctx context.Context, gr gitRunner, dir string) (*gitStatus, error) {
	root, err := gr.Git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return nil, errNoGitRepo
		}
		return nil, err
	}
	out, err := gr.Git(ctx, dir, "status", "--porcelain", "-z", "--branch")
	if err != nil {
		return nil, err
	}
	st := &gitStatus{dir: dir, root: strings.TrimSpace(root)}
	st.branch, st.ahead, st.behind, st.files = parseGitStatus(out)
	return st, nil
}

// parseGitStatus reads what git status --porcelain -z --branch says.
func parseGitStatus(out string) (branch string, ahead, behind int, files []gitFile) {
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if entry == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(entry, "## "); ok {
			branch, ahead, behind = parseGitBranch(rest)
			continue
		}
		if len(entry) < 4 {
			continue
		}
		f := gitFile{x: entry[0], y: entry[1], path: entry[3:]}
		if f.x == 'R' || f.x == 'C' || f.y == 'R' || f.y == 'C' {
			// a rename or a copy says where it came from next
			if i+1 < len(entries) {
				i++
				f.orig = entries[i]
			}
		}
		files = append(files, f)
	}
	return branch, ahead, behind, files
}

// parseGitBranch reads the branch line of a status: its name, and how far
// it is ahead of and behind its upstream.
func parseGitBranch(s string) (branch string, ahead, behind int) {
	s = strings.TrimPrefix(s, "No commits yet on ")
	counts := ""
	if i := strings.IndexByte(s, '['); i >= 0 {
		counts = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s[i:]), "["), "]")
		s = s[:i]
	}
	if i := strings.Index(s, "..."); i >= 0 {
		s = s[:i]
	}
	branch = strings.TrimSpace(s)
	if branch == "" {
		branch = "HEAD"
	}
	for _, part := range strings.Split(counts, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "ahead "):
			fmt.Sscanf(part, "ahead %d", &ahead)
		case strings.HasPrefix(part, "behind "):
			fmt.Sscanf(part, "behind %d", &behind)
		}
	}
	return branch, ahead, behind
}

// gitPrepare loads what the Git view shows when the sidebar shows a folder
// it has not asked about, or one whose answers are still missing.
func (w *Window) gitPrepare(tab *Tab) {
	b := tab.files
	gr, ok := b.src.(gitRunner)
	if b == nil || !ok || b.path == "" {
		return
	}
	g := tab.git
	if g == nil {
		g = &gitView{}
		tab.git = g
	}
	if g.dir != b.path {
		// another folder: what the view says of the one before is stale
		g.dir = b.path
		g.status, g.err = nil, ""
		g.log, g.logErr = nil, ""
		g.selection = nil
		w.gitLoad(tab, g, gr)
		w.gitLogLoad(tab, g, gr)
		return
	}
	if g.status == nil && !g.loading && g.err == "" {
		w.gitLoad(tab, g, gr)
	}
	if g.log == nil && !g.logLoading && g.logErr == "" {
		w.gitLogLoad(tab, g, gr)
	}
}

// gitReload asks again for the status and the commits.
func (w *Window) gitReload(tab *Tab) {
	b := tab.files
	if b == nil || b.path == "" {
		return
	}
	gr, ok := b.src.(gitRunner)
	if !ok {
		return
	}
	g := tab.git
	if g == nil {
		g = &gitView{dir: b.path}
		tab.git = g
	}
	w.gitLoad(tab, g, gr)
	w.gitLogLoad(tab, g, gr)
}

// gitLoad asks a source's git for the folder's status, in the background.
func (w *Window) gitLoad(tab *Tab, g *gitView, gr gitRunner) {
	g.loading, g.err = true, ""
	g.request++
	request := g.request
	dir := g.dir
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		st, err := readGitStatus(ctx, gr, dir)
		branches, _ := gr.Git(ctx, dir, "for-each-ref", "--format=%(refname:short)", "refs/heads")
		w.onMain(func() {
			if tab.git != g || g.request != request {
				return
			}
			g.loading = false
			g.branches = strings.Fields(branches)
			if err != nil {
				g.status = nil
				g.err = err.Error()
			} else {
				g.status = st
				g.syncSelection()
			}
			w.invalidate()
		})
	}()
}

// gitLogLoad asks a source's git for the branch's commits, in the
// background.
func (w *Window) gitLogLoad(tab *Tab, g *gitView, gr gitRunner) {
	g.logLoading, g.logErr = true, ""
	g.logRequest++
	request := g.logRequest
	dir := g.dir
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		out, err := gr.Git(ctx, dir, "log", "--date-order", "-n", "300",
			"--pretty=format:%H%x1f%P%x1f%D%x1f%s%x1f%an%x1f%ar%x1e")
		w.onMain(func() {
			if tab.git != g || g.logRequest != request {
				return
			}
			g.logLoading = false
			switch {
			case err != nil && strings.Contains(err.Error(), "does not have any commits yet"):
				g.log, g.logErr = nil, ""
			case err != nil:
				g.log, g.logErr = nil, err.Error()
			default:
				g.commits = parseGitLog(out)
				g.log = gitGraphRows(g.commits)
				g.logCols = graphCols(g.log)
			}
			w.invalidate()
		})
	}()
}

// gitError says what went wrong with git in the sidebar's words.
func gitError(err string) string {
	if strings.Contains(err, "not a git repository") {
		return locales.GitNoRepo()
	}
	return err
}

// gitBody is the Git view: the branch over a split of the changes, with
// the commit under them, and the branch's commits.
func (w *Window) gitBody(c *ui.Context, lk *look, tab *Tab) {
	w.gitPrepare(tab)
	g := tab.git
	switch {
	case g == nil || g.loading && g.status == nil:
		ui.Column(c).Fill().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
		return
	case g.err != "":
		ui.Column(c).Fill().Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, gitError(g.err)).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.35).MaxWidth(220)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				w.gitReload(tab)
			}
		})
		return
	}
	st := g.status

	// the branch, with how far it is from its upstream
	ui.Row(c).FillWidth().Height(32).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
		w.gitBranchButton(c, lk, tab, g, st)
		ui.Spacer(c)
		if st.ahead > 0 {
			ui.Textf(c, "↑%d", st.ahead).FontSize(10.5).FontWeight(650).
				TextColor(lk.success).Padding(1, 5).Radius(4).Background(lk.successSoft)
		}
		if st.behind > 0 {
			ui.Textf(c, "↓%d", st.behind).FontSize(10.5).FontWeight(650).
				TextColor(lk.attention).Padding(1, 5).Radius(4).Background(lk.attentionSoft)
		}
		if g.loading {
			ui.Spinner(c).Size(11, 11)
		}
	})

	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
		// the changes, and the commit that takes them
		ui.Column(c).FillWidth().Grow(w.gitShare).Basis(0).MinHeight(0).Children(func() {
			w.gitChanges(c, lk, tab, g, st)
			w.gitCommitBox(c, lk, tab, g)
		})
		w.gitSplitter(c, lk)
		// the commits of the branch
		ui.Column(c).FillWidth().Grow(1 - w.gitShare).Basis(0).MinHeight(0).Children(func() {
			w.gitCommits(c, lk, tab, g)
		})
	})
}

// gitBranchButton shows the branch, and switches it from its menu.
func (w *Window) gitBranchButton(c *ui.Context, lk *look, tab *Tab, g *gitView, st *gitStatus) {
	b := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(5).Gap(4).Label(st.branch)
	if b.Hovered() {
		b.Background(lk.rowHover)
	}
	if g.switching {
		b.Disabled(true)
	}
	b.Children(func() {
		ui.Icon(c, iconGit).Size(13, 13).TextColor(lk.textMuted)
		ui.Text(c, st.branch).FontSize(12.5).FontWeight(600).TextColor(lk.text).
			MaxWidth(150).SingleLine().Ellipsis("…")
		ui.Icon(c, iconChevronDown).Size(8, 8).TextColor(lk.textFaint)
	})
	b.Tooltip(locales.GitSwitchBranch())
	if b.Clicked() {
		if !g.branchOpen {
			g.branchQuery = ""
		}
		g.branchOpen = !g.branchOpen
	}
	ui.Popover(c, b, &g.branchOpen, func() {
		ui.Column(c).Width(240).Children(func() {
			ui.Row(c).Padding(8, 8, 4, 8).Children(func() {
				ui.SearchField(c, &g.branchQuery).FillWidth().Height(26).
					Placeholder(locales.GitSearchBranches()).Label(locales.GitSearchBranches())
			})
			ui.Scroll(c).MaxHeight(300).Children(func() {
				ui.Column(c).FillWidth().Padding(2, 6, 8, 6).Gap(1).Children(func() {
					q := strings.ToLower(strings.TrimSpace(g.branchQuery))
					shown := 0
					for _, branch := range g.branches {
						if q != "" && !strings.Contains(strings.ToLower(branch), q) {
							continue
						}
						shown++
						row := ui.Row(c).FillWidth().Height(30).Padding(0, 8).Gap(8).
							Radius(6).AlignItems(ui.Center)
						if row.Hovered() {
							row.Background(lk.rowHover)
						}
						row.Children(func() {
							ui.Icon(c, iconGit).Size(12, 12).TextColor(lk.textMuted)
							fg := lk.text
							if branch == st.branch {
								fg = lk.accent
							}
							ui.Text(c, branch).FontSize(12.5).TextColor(fg).Grow(1).
								SingleLine().Ellipsis("…")
							if branch == st.branch {
								ui.Icon(c, iconCheck).Size(12, 12).TextColor(lk.accent)
							}
						})
						if row.Clicked() {
							g.branchOpen, g.branchQuery = false, ""
							w.switchBranch(tab, g, branch)
						}
					}
					if shown == 0 {
						ui.Text(c, locales.GitNoMatch()).FontSize(12).TextColor(lk.textFaint).
							Padding(8, 8).SingleLine()
					}
				})
			})
		})
	})
}

// switchBranch checks out another branch of the repository.
func (w *Window) switchBranch(tab *Tab, g *gitView, branch string) {
	if g.switching || branch == "" {
		return
	}
	b := tab.files
	if b == nil {
		return
	}
	gr, ok := b.src.(gitRunner)
	if !ok {
		return
	}
	dir := g.dir
	if g.status != nil && g.status.root != "" {
		dir = g.status.root
	}
	g.switching = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		_, err := gr.Git(ctx, dir, "checkout", branch)
		w.onMain(func() {
			g.switching = false
			if err != nil {
				w.errMsg, w.errOpen = err.Error(), true
				return
			}
			w.showToast(locales.GitSwitched(branch), "success")
			w.gitReload(tab)
		})
	}()
}

// gitChanges is the files that changed, or say there are none.
func (w *Window) gitChanges(c *ui.Context, lk *look, tab *Tab, g *gitView, st *gitStatus) {
	if len(st.files) == 0 {
		ui.Column(c).Fill().Grow(1).Center().Padding(14).Gap(6).Children(func() {
			ui.Icon(c, iconCheck).Size(20, 20).TextColor(lk.success)
			ui.Text(c, locales.GitClean()).FontSize(12).TextColor(lk.textMuted).
				TextAlign(ui.Center).LineHeight(1.35).MaxWidth(200)
		})
		return
	}
	gr, _ := tab.files.src.(gitRunner)
	ui.Scroll(c).Fill().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 6, 6, 6).Gap(1).Children(func() {
			for _, file := range st.files {
				w.gitRow(c, lk, tab, gr, st, g, file)
			}
		})
	})
}

// gitCommitBox writes a commit of every change.
func (w *Window) gitCommitBox(c *ui.Context, lk *look, tab *Tab, g *gitView) {
	ui.Column(c).FillWidth().Shrink(0).Padding(8, 8).Gap(6).
		BorderWidth(1, 0, 0, 0).BorderColor(lk.divider).Children(func() {
		area := ui.TextArea(c, &g.message).FillWidth().Lines(2, 4).
			Placeholder(locales.GitCommitPlaceholder())
		if g.committing {
			area.ReadOnly(true)
		}
		if g.commitErr != "" {
			ui.Text(c, g.commitErr).FontSize(11).TextColor(lk.danger).LineHeight(1.3)
		}
		selected, total := g.selectedCount()
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			if total > 0 {
				label := locales.GitSelectAll()
				if selected == total {
					label = locales.GitSelectNone()
				}
				all := ui.ButtonBase(c).Height(20).Padding(0, 6).Radius(5).Label(label)
				if all.Hovered() {
					all.Background(lk.rowHover)
				}
				all.Children(func() {
					ui.Text(c, label).FontSize(10.5).TextColor(lk.textMuted).SingleLine()
				})
				all.Tooltip(label)
				if all.Clicked() {
					g.selectAll(selected != total)
				}
				ui.Textf(c, "%d/%d", selected, total).FontSize(10.5).
					TextColor(lk.textFaint).SingleLine()
			}
			ui.Spacer(c)
			b := ui.PrimaryButton(c, locales.GitCommit())
			if g.committing || strings.TrimSpace(g.message) == "" || selected == 0 {
				b.Disabled(true)
			}
			b.Tooltip(locales.GitCommitAll())
			if b.Clicked() {
				w.commitChanges(tab)
			}
		})
	})
}

// gitSplitter is the edge between the changes and the commits, which a
// drag moves.
func (w *Window) gitSplitter(c *ui.Context, lk *look) {
	h := ui.Box(c.Key("git-split")).FillWidth().Height(8).Shrink(0).
		Cursor(ui.CursorResizeRow).KeepFocus()
	if h.Hovered() || h.Pressed() {
		h.Background(lk.divider)
	}
	// Dragged says how far the pointer went across first, and down second
	if _, dy, ok := h.Dragged(); ok {
		if height := w.gitBodyHeight(c); height > 0 {
			w.gitShare = min(max(w.gitShare+dy/height, 0.2), 0.8)
		}
	} else if h.DoubleClicked() {
		w.gitShare = defaultGitShare
	}
}

// gitBodyHeight is the room the split divides.
func (w *Window) gitBodyHeight(c *ui.Context) float32 {
	_, height := c.Size()
	// the window's gap above and below, the sidebar's header and branch
	return height - 10 - 10 - 36 - 32
}

// gitCommits is the branch's commits, the latest first.
func (w *Window) gitCommits(c *ui.Context, lk *look, tab *Tab, g *gitView) {
	switch {
	case g.logLoading && g.log == nil:
		ui.Column(c).Fill().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
		return
	case g.logErr != "":
		ui.Column(c).Fill().Grow(1).Center().Padding(12).Gap(6).Children(func() {
			ui.Text(c, g.logErr).FontSize(11.5).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.3).MaxWidth(200)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				if gr, ok := tab.files.src.(gitRunner); ok {
					w.gitLogLoad(tab, g, gr)
				}
			}
		})
		return
	}
	if len(g.commits) == 0 {
		ui.Column(c).Fill().Grow(1).Center().Padding(12).Children(func() {
			ui.Text(c, locales.GitNoCommits()).FontSize(12).TextColor(lk.textMuted).
				TextAlign(ui.Center)
		})
		return
	}
	rows, cols := g.filter.shown(g.commits, g.log, g.logCols)
	w.gitFilterRow(c, lk, &g.filter.query, len(rows), len(g.commits))
	if len(rows) == 0 {
		ui.Column(c).Fill().Grow(1).Center().Padding(12).Children(func() {
			ui.Text(c, locales.GitNoMatch()).FontSize(12).TextColor(lk.textMuted).
				TextAlign(ui.Center)
		})
		return
	}
	gr, _ := tab.files.src.(gitRunner)
	ui.Scroll(c).Fill().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 6, 6, 6).Children(func() {
			for _, row := range rows {
				row := row
				w.graphRow(c, lk, row, cols,
					g.selected == row.commit.hash,
					func() { g.selected = row.commit.hash },
					func() {
						if gr != nil {
							w.openCommit(tab.files.src, gr, g.dir, row.commit.hash)
						}
					})
			}
		})
	})
}

// gitRow is one changed file: whether the commit takes it, its state, and
// where it is.
func (w *Window) gitRow(c *ui.Context, lk *look, tab *Tab, gr gitRunner, st *gitStatus, g *gitView, file gitFile) {
	selected := g.selection[file.path]
	row := ui.Row(c).FillWidth().Height(28).Padding(0, 6).Gap(7).Radius(6).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(lk.rowHover)
		row.Cursor(ui.CursorPointer)
	}
	row.Tooltip(file.path)
	deleted := file.x == 'D' || file.y == 'D'
	row.ContextMenu(func(m *ui.Menu) {
		if gr != nil && m.Item(locales.GitShowChanges()).Chosen() {
			w.openDiff(tab.files.src, gr, st.root, file)
		}
		if !deleted && m.Item(locales.FilesOpen()).Chosen() {
			w.openFileTab(tab.files.src, tab.files.src.Join(st.root, file.path), pathBase(file.path))
		}
		m.Separator()
		if m.Item(locales.FilesCopyPath()).Chosen() {
			c.WriteClipboard(tab.files.src.Join(st.root, file.path))
		}
	})
	added := false
	row.Children(func() {
		box := ui.Box(c).Size(14, 14).Radius(4).Center().Border(1, lk.cardBorder)
		if selected {
			box.Background(lk.accent).BorderColor(lk.accent)
			box.Children(func() {
				ui.Icon(c, iconCheck).Size(9, 9).TextColor(ui.Hex("#ffffff"))
			})
		}
		fg, tip := gitTint(lk, file)
		ui.Text(c, gitState(file)).FontSize(10.5).FontWeight(700).TextColor(fg).
			Width(20).SingleLine().Tooltip(tip)
		name := lk.text
		if !selected {
			name = lk.textMuted
		}
		ui.Text(c, file.path).FontSize(12.5).TextColor(name).Grow(1).
			SingleLine().Ellipsis("…")
		if isUntracked(file) && gr != nil {
			b := ui.ButtonBase(c).Size(18, 18).Radius(4).Center().Label(locales.GitAddFile())
			if b.Hovered() {
				b.Background(lk.chip)
			}
			b.Children(func() {
				ui.Icon(c, iconPlus).Size(11, 11).TextColor(lk.textMuted)
			})
			b.Tooltip(locales.GitAddFile())
			if b.Clicked() {
				added = true
				w.stageFile(tab, gr, st, file)
			}
		}
	})
	if row.DoubleClicked() && gr != nil {
		w.openDiff(tab.files.src, gr, st.root, file)
	} else if row.Clicked() && !added {
		g.toggle(file.path)
	}
}

// isUntracked reports whether git tracks no version of a file yet.
func isUntracked(f gitFile) bool { return f.x == '?' && f.y == '?' }

// stageFile adds a file Git does not track to the index, which then shows
// as added.
func (w *Window) stageFile(tab *Tab, gr gitRunner, st *gitStatus, file gitFile) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		if _, err := gr.Git(ctx, st.root, "add", "--", file.path); err != nil {
			w.onMain(func() {
				w.errMsg, w.errOpen = err.Error(), true
			})
			return
		}
		w.onMain(func() {
			if tab.git != nil {
				w.gitReload(tab)
			}
		})
	}()
}

// toggle takes a changed file in or out of the commit.
func (g *gitView) toggle(path string) {
	if g.selection == nil {
		g.selection = map[string]bool{}
	}
	g.selection[path] = !g.selection[path]
}

// syncSelection takes every changed file the view has not seen yet.
func (g *gitView) syncSelection() {
	if g.selection == nil {
		g.selection = map[string]bool{}
	}
	if g.status == nil {
		return
	}
	for _, f := range g.status.files {
		if _, ok := g.selection[f.path]; !ok {
			g.selection[f.path] = true
		}
	}
}

// selectedCount is how many changed files the commit takes, of how many.
func (g *gitView) selectedCount() (selected, total int) {
	if g.status == nil {
		return 0, 0
	}
	for _, f := range g.status.files {
		total++
		if g.selection[f.path] {
			selected++
		}
	}
	return selected, total
}

// selectAll takes every changed file, or none.
func (g *gitView) selectAll(on bool) {
	if g.selection == nil {
		g.selection = map[string]bool{}
	}
	if g.status == nil {
		return
	}
	for _, f := range g.status.files {
		g.selection[f.path] = on
	}
}

// commitPaths are the paths of the files the commit takes, a rename's old
// path among them.
func (g *gitView) commitPaths() []string {
	if g.status == nil {
		return nil
	}
	var paths []string
	for _, f := range g.status.files {
		if !g.selection[f.path] {
			continue
		}
		if f.orig != "" {
			paths = append(paths, f.orig)
		}
		paths = append(paths, f.path)
	}
	return paths
}

// commitChanges stages every change and commits it with the message the
// view holds.
func (w *Window) commitChanges(tab *Tab) {
	g := tab.git
	b := tab.files
	if g == nil || b == nil || g.committing {
		return
	}
	message := strings.TrimSpace(g.message)
	if message == "" {
		return
	}
	gr, ok := b.src.(gitRunner)
	if !ok {
		return
	}
	paths := g.commitPaths()
	if len(paths) == 0 {
		g.commitErr = locales.GitNothingSelected()
		return
	}
	dir := g.dir
	if g.status != nil && g.status.root != "" {
		dir = g.status.root
	}
	g.committing, g.commitErr = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		if _, err := gr.Git(ctx, dir, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
			w.commitFailed(tab, g, err)
			return
		}
		if _, err := gr.Git(ctx, dir, append([]string{"commit", "-m", message, "--"}, paths...)...); err != nil {
			w.commitFailed(tab, g, err)
			return
		}
		w.onMain(func() {
			if tab.git != g {
				return
			}
			g.committing, g.message = false, ""
			w.showToast(locales.GitCommitted(), "success")
			w.gitReload(tab)
		})
	}()
}

// commitFailed shows what went wrong with a commit, under its message.
func (w *Window) commitFailed(tab *Tab, g *gitView, err error) {
	w.onMain(func() {
		if tab.git != g {
			return
		}
		g.committing = false
		g.commitErr = err.Error()
		w.invalidate()
	})
}

// gitState is a changed file's two letters, as git status says them: an
// untracked file, git's "??", says U, as editors show it, rather than the
// question marks that read as a missing glyph.
func gitState(f gitFile) string {
	if f.x == '?' && f.y == '?' {
		return "U"
	}
	return strings.TrimSpace(string([]byte{f.x, f.y}))
}

// gitTint is a changed file's color and what its state means.
func gitTint(lk *look, f gitFile) (ui.Color, string) {
	switch {
	case f.x == '?' && f.y == '?':
		return lk.textMuted, locales.GitUntracked()
	case f.x == 'U' || f.y == 'U':
		return lk.danger, locales.GitConflict()
	case f.y != ' ':
		return lk.attention, locales.GitModified()
	}
	return lk.success, locales.GitStaged()
}

// pathBase is the name of a file in a repository's path.
func pathBase(file string) string {
	if i := strings.LastIndexByte(file, '/'); i >= 0 {
		return file[i+1:]
	}
	return file
}

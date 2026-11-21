package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// gitCommit is one commit of the graph.
type gitCommit struct {
	hash    string
	subject string
	author  string
	when    string
	parents []string
	refs    []string
}

// gitGraphRow is a commit, the lane it is drawn in, the lanes that come into
// the row and leave it, and the lanes its other parents join.
type gitGraphRow struct {
	commit *gitCommit
	col    int
	in     []string
	out    []string
	joins  []int
}

// graphTab is a repository's commits in the pane.
type graphTab struct {
	src      fileSource
	gr       gitRunner
	dir      string
	rows     []gitGraphRow
	cols     int
	commits  []*gitCommit
	filter   commitFilter
	selected string // the commit the graph shows as chosen
	loading  bool
	err      string
	request  uint64
}

// graphRowHeight is the height of a commit's row, and graphLane the room a
// lane of its graph takes.
const (
	graphRowHeight = 30
	graphLane      = 12
	// defaultGitShare is the part of the Git view the changes take, the
	// commits taking the rest.
	defaultGitShare = 0.58
)

// openGraph shows a repository's commits in a tab of the pane, or the tab
// it has.
func (w *Window) openGraph(src fileSource, gr gitRunner, dir string) {
	if g := w.paneGraph(src, dir); g != nil {
		for i, tab := range w.paneTabs {
			if tab.graph == g {
				w.paneSel = i
				break
			}
		}
		w.syncMenus()
		w.invalidate()
		return
	}
	g := &graphTab{src: src, gr: gr, dir: dir, loading: true}
	w.paneTabs = append(w.paneTabs, &paneTab{graph: g})
	w.paneSel = len(w.paneTabs) - 1
	w.syncMenus()
	w.graphLoad(g)
}

// paneGraph is the graph of a folder the pane holds, or nil.
func (w *Window) paneGraph(src fileSource, dir string) *graphTab {
	for _, t := range w.paneTabs {
		if t.graph != nil && t.graph.src == src && t.graph.dir == dir {
			return t.graph
		}
	}
	return nil
}

// holdsGraph reports whether the pane still holds the graph.
func (w *Window) holdsGraph(g *graphTab) bool {
	for _, t := range w.paneTabs {
		if t.graph == g {
			return true
		}
	}
	return false
}

// graphLoad reads a repository's commits in the background.
func (w *Window) graphLoad(g *graphTab) {
	g.loading, g.err = true, ""
	g.request++
	request := g.request
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		out, err := g.gr.Git(ctx, g.dir, "log", "--all", "--date-order", "-n", "300",
			"--pretty=format:%H%x1f%P%x1f%D%x1f%s%x1f%an%x1f%ar%x1e")
		w.onMain(func() {
			if !w.holdsGraph(g) || g.request != request {
				return
			}
			g.loading = false
			switch {
			case err != nil && strings.Contains(err.Error(), "does not have any commits yet"):
				g.err = locales.GitNoCommits()
			case err != nil:
				g.err = err.Error()
			default:
				g.commits = parseGitLog(out)
				g.rows = gitGraphRows(g.commits)
				g.cols = graphCols(g.rows)
			}
			w.invalidate()
		})
	}()
}

// parseGitLog reads what git log --pretty=format:... says.
func parseGitLog(out string) []*gitCommit {
	var commits []*gitCommit
	for _, entry := range strings.Split(out, "\x1e") {
		entry = strings.TrimLeft(entry, "\r\n")
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "\x1f")
		if len(parts) < 6 {
			continue
		}
		c := &gitCommit{hash: parts[0], subject: parts[3], author: parts[4], when: parts[5]}
		if parts[1] != "" {
			c.parents = strings.Fields(parts[1])
		}
		for _, ref := range strings.Split(parts[2], ", ") {
			if ref = strings.TrimSpace(ref); ref != "" {
				c.refs = append(c.refs, ref)
			}
		}
		commits = append(commits, c)
	}
	return commits
}

// gitGraphRows puts commits in lanes, the first parent continuing in its
// commit's lane and the others joining the lanes that wait for them, or new
// ones.
func gitGraphRows(commits []*gitCommit) []gitGraphRow {
	return gitGraphRowsKeeping(commits, nil)
}

// gitGraphRowsKeeping puts commits in lanes, and stops a lane at a parent
// that is not in keep, as a filter that leaves commits out needs.
func gitGraphRowsKeeping(commits []*gitCommit, keep map[string]bool) []gitGraphRow {
	kept := func(hash string) bool { return keep == nil || keep[hash] }
	var lanes []string
	rows := make([]gitGraphRow, 0, len(commits))
	for _, c := range commits {
		col := -1
		var dups []int
		for i, hash := range lanes {
			if hash != c.hash {
				continue
			}
			if col < 0 {
				col = i
				continue
			}
			dups = append(dups, i) // a lane the commit already has
		}
		if col < 0 {
			if col = freeLane(lanes); col < 0 {
				col = len(lanes)
				lanes = append(lanes, "")
			}
			lanes[col] = c.hash
		}
		row := gitGraphRow{commit: c, col: col, in: append([]string(nil), lanes...)}
		for _, i := range dups {
			lanes[i] = ""
			row.joins = append(row.joins, i)
		}
		lanes[col] = ""
		if len(c.parents) > 0 && kept(c.parents[0]) {
			lanes[col] = c.parents[0]
		}
		for _, parent := range c.parents[min(1, len(c.parents)):] {
			if !kept(parent) {
				continue
			}
			j := laneOf(lanes, parent)
			if j < 0 {
				if j = freeLane(lanes); j < 0 {
					j = len(lanes)
					lanes = append(lanes, "")
				}
				lanes[j] = parent
			}
			row.joins = append(row.joins, j)
		}
		row.out = append([]string(nil), lanes...)
		rows = append(rows, row)
		for len(lanes) > 0 && lanes[len(lanes)-1] == "" {
			lanes = lanes[:len(lanes)-1]
		}
	}
	return rows
}

// freeLane is the first lane that waits for nothing, or -1.
func freeLane(lanes []string) int {
	for i, hash := range lanes {
		if hash == "" {
			return i
		}
	}
	return -1
}

// laneOf is the lane that waits for a commit, or -1.
func laneOf(lanes []string, hash string) int {
	for i, h := range lanes {
		if h == hash {
			return i
		}
	}
	return -1
}

// graphBody is the Git graph: the commits of the repository, in lanes,
// or those a filter keeps.
func (w *Window) graphBody(c *ui.Context, lk *look, g *graphTab) {
	switch {
	case g.loading && g.rows == nil:
		ui.Column(c).Fill().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
		return
	case g.err != "":
		ui.Column(c).Fill().Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, g.err).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.35).MaxWidth(320)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				w.graphLoad(g)
			}
		})
		return
	}
	rows, cols := g.filter.shown(g.commits, g.rows, g.cols)
	w.gitFilterRow(c, lk, &g.filter.query, len(rows), len(g.commits))
	switch {
	case len(g.commits) == 0:
		w.filesMessage(c, lk, locales.GitNoCommits(), "")
	case len(rows) == 0:
		w.filesMessage(c, lk, locales.GitNoMatch(), "")
	default:
		ui.Scroll(c).Fill().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(0, 6, 8, 6).Children(func() {
				for _, row := range rows {
					row := row
					w.graphRow(c, lk, row, cols,
						g.selected == row.commit.hash,
						func() { g.selected = row.commit.hash },
						func() {
							w.openCommit(g.src, g.gr, g.dir, row.commit.hash)
						})
				}
			})
		})
	}
}

// gitFilterRow is a filter over commits, and how many of them it keeps.
func (w *Window) gitFilterRow(c *ui.Context, lk *look, query *string, shown, total int) {
	ui.Row(c).FillWidth().Height(30).Shrink(0).Padding(0, 6).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
		field := ui.SearchField(c, query).Grow(1).Height(24).
			Placeholder(locales.GitFilterCommits()).Label(locales.GitFilterCommits())
		field.Tooltip(locales.GitFilterTip())
		if *query != "" {
			ui.Textf(c, "%d/%d", shown, total).FontSize(10.5).TextColor(lk.textFaint).SingleLine()
		}
	})
}

// commitFilter is a query over commits and the rows it keeps, made once
// for the query.
type commitFilter struct {
	query    string
	forQuery string
	applied  bool
	rows     []gitGraphRow
	cols     int
}

// shown is the rows a filter shows and their lanes: all of them while the
// query is empty.
func (f *commitFilter) shown(commits []*gitCommit, all []gitGraphRow, allCols int) ([]gitGraphRow, int) {
	if f.query == "" {
		return all, allCols
	}
	if !f.applied || f.forQuery != f.query {
		kept := filterCommits(commits, f.query)
		f.rows = gitGraphRowsKeeping(kept, commitSet(kept))
		f.cols = graphCols(f.rows)
		f.forQuery, f.applied = f.query, true
	}
	return f.rows, f.cols
}

// filterCommits keeps the commits a query matches, in their order.
func filterCommits(commits []*gitCommit, query string) []*gitCommit {
	var kept []*gitCommit
	for _, c := range commits {
		if commitMatches(c, query) {
			kept = append(kept, c)
		}
	}
	return kept
}

// commitMatches reports whether a commit matches every term of a query,
// which is its subject, its author and its hash, or the same after
// author:, msg: or hash:.
func commitMatches(c *gitCommit, query string) bool {
	subject := strings.ToLower(c.subject)
	author := strings.ToLower(c.author)
	hash := strings.ToLower(c.hash)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		var ok bool
		switch {
		case strings.HasPrefix(term, "author:"):
			ok = strings.Contains(author, strings.TrimPrefix(term, "author:"))
		case strings.HasPrefix(term, "msg:"):
			ok = strings.Contains(subject, strings.TrimPrefix(term, "msg:"))
		case strings.HasPrefix(term, "subject:"):
			ok = strings.Contains(subject, strings.TrimPrefix(term, "subject:"))
		case strings.HasPrefix(term, "hash:"):
			ok = strings.Contains(hash, strings.TrimPrefix(term, "hash:"))
		default:
			ok = strings.Contains(subject, term) || strings.Contains(author, term) ||
				strings.Contains(hash, term)
		}
		if !ok {
			return false
		}
	}
	return true
}

// commitSet is the set of commits' hashes, for lanes a filter keeps.
func commitSet(commits []*gitCommit) map[string]bool {
	set := make(map[string]bool, len(commits))
	for _, c := range commits {
		set[c.hash] = true
	}
	return set
}

// graphCols is the lanes the rows of a graph take.
func graphCols(rows []gitGraphRow) int {
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row.in), len(row.out))
		for _, j := range row.joins {
			cols = max(cols, j+1)
		}
	}
	return cols
}

// graphButtons are the graph's own actions: reading the commits again.
func (w *Window) graphButtons(c *ui.Context, lk *look, g *graphTab) {
	w.rowButton(c, lk, iconRefresh, locales.FilesRefresh(), func() { w.graphLoad(g) })
}

// graphRow is one commit of the graph: its lanes, its refs and what it says.
func (w *Window) graphRow(c *ui.Context, lk *look, row gitGraphRow, cols int, selected bool, onSelect, open func()) {
	graphWidth := float32(cols*graphLane) + 10
	el := ui.Row(c).FillWidth().Height(graphRowHeight).Shrink(0).Padding(0, 6).Gap(8).
		Radius(6).AlignItems(ui.Center).Tooltip(row.commit.tip())
	el.ContextMenu(func(m *ui.Menu) {
		if open != nil && m.Item(locales.GitShowCommit()).Chosen() {
			open()
		}
		if m.Item(locales.GitCopyHash()).Chosen() {
			c.WriteClipboard(row.commit.hash)
		}
	})
	if selected {
		el.Background(lk.accentSoft)
	} else if el.Hovered() {
		el.Background(lk.rowHover)
	}
	el.Cursor(ui.CursorPointer)
	el.Children(func() {
		ui.Box(c).Width(graphWidth).FillHeight().Children(func() {
			graphLanes(c, lk, row, cols)
		})
		ui.Text(c, row.commit.subject).FontSize(12.5).TextColor(lk.text).Grow(1).
			SingleLine().Ellipsis("…")
		if refs, more := graphRefs(row.commit.refs); len(refs) > 0 {
			ui.Row(c).MaxWidthPercent(46).Clip().Shrink(1).Gap(4).AlignItems(ui.Center).
				Children(func() {
					for _, ref := range refs {
						w.gitRefChip(c, lk, ref)
					}
					if more > 0 {
						ui.Textf(c, "+%d", more).FontSize(10).TextColor(lk.textFaint).SingleLine()
					}
				})
		}
		ui.Text(c, row.commit.short()).FontSize(10.5).TextColor(lk.textFaint).SingleLine()
	})
	if el.DoubleClicked() {
		if onSelect != nil {
			onSelect()
		}
		if open != nil {
			open()
		}
	} else if el.Clicked() && onSelect != nil {
		onSelect()
	}
}

// tip says what a commit does not show: who made it, and when.
func (c *gitCommit) tip() string {
	return c.short() + " · " + c.author + " · " + c.when
}

// short is a commit's hash as git shows it.
func (c *gitCommit) short() string {
	if len(c.hash) > 7 {
		return c.hash[:7]
	}
	return c.hash
}

// gitRefChip is a branch, a tag or HEAD on a commit, in as few letters as
// say which, with the whole ref as its tooltip.
func (w *Window) gitRefChip(c *ui.Context, lk *look, ref string) {
	fg, bg := lk.textMuted, lk.chip
	switch {
	case strings.HasPrefix(ref, "tag: "):
		fg, bg = lk.attention, lk.attentionSoft
	case ref == "HEAD" || strings.HasPrefix(ref, "HEAD -> "):
		fg, bg = lk.accent, lk.accentSoft
	}
	ui.Text(c, refLabel(ref)).FontSize(10).FontWeight(600).TextColor(fg).Padding(1, 5).
		Radius(4).Background(bg).MinWidth(26).MaxWidth(96).SingleLine().Ellipsis("…").
		Tooltip(ref)
}

// refLabel shortens a ref as a chip says it: a tag without its prefix, and
// HEAD as the branch it is on. A remote keeps its name, which tells it from
// the branch of the same name here.
func refLabel(ref string) string {
	switch {
	case strings.HasPrefix(ref, "HEAD -> "):
		return strings.TrimPrefix(ref, "HEAD -> ")
	case strings.HasPrefix(ref, "tag: "):
		return strings.TrimPrefix(ref, "tag: ")
	}
	return ref
}

// graphRefs picks the refs a commit's row shows: those of its own branches
// and tags before the remote ones, at most two, and how many are left out.
func graphRefs(refs []string) (shown []string, more int) {
	var local, remote []string
	for _, ref := range refs {
		if strings.HasPrefix(ref, "HEAD") {
			local = append([]string{ref}, local...)
			continue
		}
		if strings.Contains(ref, "/") {
			remote = append(remote, ref)
			continue
		}
		local = append(local, ref)
	}
	all := append(local, remote...)
	if len(all) > 2 {
		return all[:2], len(all) - 2
	}
	return all, 0
}

// graphLanes draws a commit's lanes: through lines, the joins of a merge,
// and the commit itself.
func graphLanes(c *ui.Context, lk *look, row gitGraphRow, cols int) {
	colors := [4]ui.Color{lk.accent, lk.success, lk.attention, lk.danger}
	color := func(lane int) ui.Color { return colors[lane%len(colors)] }
	laneX := func(lane int) float32 { return float32(lane*graphLane) + 5 }
	const height = float32(graphRowHeight)
	centerY := height / 2

	for lane := 0; lane < cols; lane++ {
		in := lane < len(row.in) && row.in[lane] != ""
		out := lane < len(row.out) && row.out[lane] != ""
		x := laneX(lane)
		switch {
		case lane == row.col:
			if in {
				graphLine(c, x, 0, 1, centerY, color(lane))
			}
			if out {
				graphLine(c, x, centerY, 1, height-centerY, color(lane))
			}
		case in && out:
			graphLine(c, x, 0, 1, height, color(lane))
		case in:
			graphLine(c, x, 0, 1, centerY, color(lane))
		case out:
			graphLine(c, x, centerY, 1, height-centerY, color(lane))
		}
	}

	for _, lane := range row.joins {
		x0, x1 := laneX(row.col), laneX(lane)
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		graphLine(c, x0, centerY, x1-x0+1, 1, color(lane))
	}

	ui.Box(c).Absolute().Left(laneX(row.col)-3.5).Top(centerY-3.5).Size(7, 7).
		Radius(3.5).Background(color(row.col))
}

// graphLine is a hairline of the graph, at x and y in its row.
func graphLine(c *ui.Context, x, y, width, height float32, color ui.Color) {
	ui.Box(c).Absolute().Left(x).Top(y).Width(width).Height(height).Background(color)
}

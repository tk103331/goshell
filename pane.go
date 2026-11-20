package main

import (
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// defaultPaneShare is the part of the middle area the right pane takes
// when it first shows.
const defaultPaneShare = 0.48

// paneTab is one tab of the right pane: a file in the editor or a web
// page.
type paneTab struct {
	ed     *editorFile
	web    *webTab
	diff   *diffFile
	graph  *graphTab
	commit *commitDiff
	image  *imagePreview
}

// title is what the tab bar and the menus call the tab.
func (t *paneTab) title() string {
	switch {
	case t.ed != nil:
		return t.ed.name
	case t.web != nil:
		return t.web.title
	case t.diff != nil:
		return t.diff.name
	case t.graph != nil:
		return locales.GitGraph()
	case t.commit != nil:
		if t.commit.subject != "" {
			return t.commit.subject
		}
		return t.commit.short
	case t.image != nil:
		return t.image.name
	}
	return ""
}

// tooltip says where the tab's file or page is.
func (t *paneTab) tooltip() string {
	switch {
	case t.ed != nil:
		return t.ed.path
	case t.web != nil:
		return t.web.url
	case t.diff != nil:
		return t.diff.path
	case t.graph != nil:
		return t.graph.dir
	case t.commit != nil:
		return t.commit.hash
	case t.image != nil:
		if t.image.width > 0 && t.image.height > 0 {
			return t.image.path + " · " + strconv.Itoa(int(t.image.width)) + " × " +
				strconv.Itoa(int(t.image.height))
		}
		return t.image.path
	}
	return ""
}

// webTab is a web page in the right pane.
type webTab struct {
	wv      *mygo.WebView
	url     string
	title   string
	loading bool
	err     string
}

// webTitle names a page by its host until its title comes.
func webTitle(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// selectedPane is the pane tab that shows.
func (w *Window) selectedPane() *paneTab {
	if w.paneSel < 0 || w.paneSel >= len(w.paneTabs) {
		return nil
	}
	return w.paneTabs[w.paneSel]
}

// openEditor opens a file in a pane tab, or shows the tab it has.
func (w *Window) openEditor(src fileSource, file, name string) {
	if t := w.paneEditor(src, file); t != nil {
		for i, tab := range w.paneTabs {
			if tab.ed == t {
				w.paneSel = i
				break
			}
		}
		w.syncMenus()
		w.invalidate()
		return
	}
	f := &editorFile{src: src, path: file, name: name, loading: true, wrap: true}
	if kindOf(name) == kindMarkdown {
		f.md = newMarkdownOptions()
	}
	w.paneTabs = append(w.paneTabs, &paneTab{ed: f})
	w.paneSel = len(w.paneTabs) - 1
	w.syncMenus()
	w.loadEditorFile(f)
}

// paneEditor is the editor of a file the pane holds, or nil.
func (w *Window) paneEditor(src fileSource, file string) *editorFile {
	for _, t := range w.paneTabs {
		if t.ed != nil && t.ed.src == src && t.ed.path == file {
			return t.ed
		}
	}
	return nil
}

// openWeb opens a web page in a pane tab, or shows the tab it has.
func (w *Window) openWeb(raw string) {
	for i, t := range w.paneTabs {
		if t.web != nil && t.web.url == raw {
			w.paneSel = i
			w.syncMenus()
			w.invalidate()
			return
		}
	}
	web := &webTab{url: raw, title: webTitle(raw), loading: true}
	tab := &paneTab{web: web}
	if w.win != nil {
		wv, err := w.win.NewWebView(mygo.WebViewOptions{URL: raw})
		if err != nil {
			web.loading, web.err = false, err.Error()
		} else {
			web.wv = wv
			w.watchWeb(web)
		}
	} else {
		web.loading, web.err = false, "a web view needs a window"
	}
	w.paneTabs = append(w.paneTabs, tab)
	w.paneSel = len(w.paneTabs) - 1
	w.syncMenus()
	w.invalidate()
}

// watchWeb follows a page's title, address and load into its tab.
func (w *Window) watchWeb(web *webTab) {
	page := web.wv.Page()
	page.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		// the window keeps the session's name, not the page's
		e.PreventDefault()
		w.onMain(func() {
			if e.Title != "" {
				web.title = e.Title
				w.invalidate()
			}
		})
	})
	page.OnDidNavigate(func(u string) {
		w.onMain(func() {
			web.url = u
			w.invalidate()
		})
	})
	page.OnDidFinishLoad(func() {
		w.onMain(func() {
			web.loading, web.err = false, ""
			w.invalidate()
		})
	})
	page.OnDidFailLoad(func(e *mygo.LoadError) {
		w.onMain(func() {
			web.loading, web.err = false, e.Description
			w.invalidate()
		})
	})
}

// closePaneTab closes a tab of the pane, asking first when its file has
// changes.
func (w *Window) closePaneTab(i int) {
	if i < 0 || i >= len(w.paneTabs) {
		return
	}
	tab := w.paneTabs[i]
	close := func() { w.removePaneTab(i) }
	if tab.ed != nil && tab.ed.modified() {
		w.confirm = &confirmDialog{
			title:   locales.EditorDiscardTitle(),
			message: locales.EditorDiscardMsg(tab.ed.name),
			ok:      locales.BtnDiscard(),
			fn:      close,
		}
		return
	}
	close()
}

// removePaneTab takes a tab out of the pane, and destroys its web view.
func (w *Window) removePaneTab(i int) {
	if i < 0 || i >= len(w.paneTabs) {
		return
	}
	tab := w.paneTabs[i]
	if tab.web != nil && tab.web.wv != nil {
		tab.web.wv.Destroy()
	}
	w.paneTabs = append(w.paneTabs[:i], w.paneTabs[i+1:]...)
	if w.paneSel >= len(w.paneTabs) {
		w.paneSel = len(w.paneTabs) - 1
	}
	if w.paneSel > i {
		w.paneSel--
	}
	if w.paneSel < 0 {
		w.paneSel = 0
	}
	w.syncMenus()
	w.invalidate()
}

// closePanes closes every tab of the pane, as the window closes.
func (w *Window) closePanes() {
	for _, tab := range w.paneTabs {
		if tab.web != nil && tab.web.wv != nil {
			tab.web.wv.Destroy()
		}
	}
	w.paneTabs = nil
	w.paneSel = 0
}

// syncMenus enables the file commands the window can run now.
func (w *Window) syncMenus() {
	if w.menuSave == nil {
		return
	}
	tab := w.selectedPane()
	save := tab != nil && tab.ed != nil && tab.ed.buf != nil && !tab.ed.readOnly
	w.menuSave.SetEnabled(save)
}

// openLink opens what a Command+click on the terminal found: a web page in
// the pane, or a file it printed in the editor.
func (w *Window) openLink(tab *Tab, link string) bool {
	if u, err := url.Parse(link); err == nil && u.Host != "" &&
		(u.Scheme == "http" || u.Scheme == "https") {
		w.openWeb(link)
		return true
	}
	return w.openPathLink(tab, link)
}

// openPathLink opens a file path a command printed in the editor, when the
// sidebar can read it.
func (w *Window) openPathLink(tab *Tab, link string) bool {
	b := tab.files
	if b == nil || b.src == nil || link == "" {
		return false
	}
	if u, err := url.Parse(link); err == nil && len(u.Scheme) > 1 {
		// mailto:, file: and other schemes go to the system
		return false
	}
	file := stripLineCol(link)
	absolute := strings.HasPrefix(file, "/")
	if !absolute && runtime.GOOS == "windows" {
		absolute = filepath.IsAbs(file)
	}
	if !absolute {
		dir := b.path
		if tab.term != nil {
			if d := tab.term.Dir(); d != "" {
				dir = d
			}
		}
		file = b.src.Join(dir, file)
	}
	w.openFileTab(b.src, file, path.Base(file))
	return true
}

// stripLineCol takes a :line and :line:col suffix off a path, as editors
// print them.
func stripLineCol(file string) string {
	s := file
	if i := strings.LastIndexByte(s, ':'); i > 0 && isDigits(s[i+1:]) {
		s = s[:i]
		if j := strings.LastIndexByte(s, ':'); j > 0 && isDigits(s[j+1:]) {
			s = s[:j]
		}
	}
	return s
}

// isDigits reports whether s is one or more digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// middleWidth is the width of the middle area, the row the terminal and the
// pane share between the sidebars.
func (w *Window) middleWidth(c *ui.Context) float32 {
	width, _ := c.Size()
	if w.showSidebar {
		width -= w.sidebar + 8
	}
	if w.showFiles {
		width -= w.filesWidth + 8
	}
	return width
}

// paneHandle is the edge between the terminal and the pane, which a drag
// moves.
func (w *Window) paneHandle(c *ui.Context, lk *look) {
	h := ui.Box(c.Key("pane-resize")).Width(8).FillHeight().
		Cursor(ui.CursorResizeColumn).KeepFocus()
	if h.Hovered() || h.Pressed() {
		h.Background(lk.divider)
	}
	if dx, _, ok := h.Dragged(); ok {
		if middle := w.middleWidth(c); middle > 0 {
			w.paneShare = min(max(w.paneShare-dx/middle, 0.2), 0.8)
		}
	} else if h.DoubleClicked() {
		w.paneShare = defaultPaneShare
	}
}

// paneView is the right pane: the tabs of files and pages, and the one
// that shows.
func (w *Window) paneView(c *ui.Context, lk *look) {
	right := float32(10)
	if w.showFiles {
		right = 0
	}
	ui.Column(c).FillHeight().Grow(w.paneShare).Basis(0).MinWidth(0).
		Padding(0, right, 10, 0).Children(func() {
		card := w.card(c, lk)
		card.Children(func() {
			w.paneTabBar(c, lk)
			tab := w.selectedPane()
			switch {
			case tab == nil:
			case tab.ed != nil:
				w.editorBody(c, lk, tab.ed)
			case tab.web != nil:
				w.webBody(c, lk, tab.web)
			case tab.diff != nil:
				w.diffBody(c, lk, tab.diff)
			case tab.graph != nil:
				w.graphBody(c, lk, tab.graph)
			case tab.commit != nil:
				w.commitBody(c, lk, tab.commit)
			case tab.image != nil:
				w.imageBody(c, lk, tab.image)
			}
		})
	})
}

// paneTabBar draws the pane's tabs, and the actions of the one that shows.
func (w *Window) paneTabBar(c *ui.Context, lk *look) {
	ui.Row(c).FillWidth().Height(36).Shrink(0).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
		ui.ScrollHorizontal(c).FillHeight().Grow(1).Shrink(1).Children(func() {
			ui.Row(c).Height(36).Padding(2, 4).Gap(2).AlignItems(ui.Center).Children(func() {
				for i, tab := range w.paneTabs {
					w.paneTab(c, lk, i, tab)
				}
			})
		})
		if tab := w.selectedPane(); tab != nil {
			ui.Row(c).Padding(0, 6).Gap(2).AlignItems(ui.Center).Children(func() {
				switch {
				case tab.ed != nil:
					if tab.ed.md != nil {
						w.editorMarkdownButton(c, lk, tab.ed)
					}
					if tab.ed.md == nil || tab.ed.source {
						w.editorWrapButton(c, lk, tab.ed)
					}
					w.editorExternalButton(c, lk, tab.ed)
				case tab.web != nil:
					w.webButtons(c, lk, tab.web)
				case tab.diff != nil:
					w.diffButtons(c, lk, tab.diff)
				case tab.graph != nil:
					w.graphButtons(c, lk, tab.graph)
				case tab.commit != nil:
					w.commitButtons(c, lk, tab.commit)
				case tab.image != nil:
					w.imageButtons(c, lk, tab.image)
				}
			})
		}
		// the middle area's right pane shows the file sidebar's button
		ui.Row(c).Padding(0, 4).AlignItems(ui.Center).Children(func() {
			w.rowButton(c, lk, iconPanelRight, locales.TooltipToggleFiles(), w.toggleFiles)
		})
	})
}

// paneTab is one tab of the pane: its file or page, and its close button.
func (w *Window) paneTab(c *ui.Context, lk *look, i int, tab *paneTab) {
	active := i == w.paneSel
	b := ui.ButtonBase(c).Height(28).Padding(0, 6).Radius(6).Gap(5).Label(tab.title())
	if active {
		b.Background(lk.chip)
	} else if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		fg := lk.textMuted
		if active {
			fg = lk.text
		}
		switch {
		case tab.web != nil && tab.web.loading:
			ui.Spinner(c).Size(11, 11)
		case tab.web != nil:
			ui.Icon(c, iconGlobe).Size(12, 12).TextColor(fg)
		case tab.diff != nil:
			ui.Icon(c, iconDiff).Size(12, 12).TextColor(fg)
		case tab.graph != nil:
			ui.Icon(c, iconGraph).Size(12, 12).TextColor(fg)
		case tab.commit != nil:
			ui.Icon(c, iconCommit).Size(12, 12).TextColor(fg)
		case tab.image != nil:
			ui.Icon(c, iconImage).Size(12, 12).TextColor(fg)
		default:
			ui.Icon(c, iconFile).Size(12, 12).TextColor(fg)
		}
		ui.Text(c, tab.title()).FontSize(12).TextColor(lk.text).MaxWidth(180).
			SingleLine().Ellipsis("…")
		if tab.ed != nil && tab.ed.modified() {
			ui.Text(c, "●").FontSize(8).TextColor(lk.accent)
		}
		close := ui.ButtonBase(c).Size(16, 16).Radius(4).Center().
			Label(locales.PaneCloseTab())
		if close.Hovered() {
			close.Background(lk.rowHover)
		}
		close.Children(func() {
			ui.Icon(c, iconClose).Size(9, 9).TextColor(lk.textFaint)
		})
		if close.Clicked() {
			w.closePaneTab(i)
		}
	})
	b.Tooltip(tab.tooltip())
	if b.Clicked() {
		w.paneSel = i
		w.syncMenus()
	}
	b.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.PaneCloseTab()).Chosen() {
			w.closePaneTab(i)
		}
	})
}

// webButtons are the page's own actions: back, forward, reload, and the
// system's browser.
func (w *Window) webButtons(c *ui.Context, lk *look, web *webTab) {
	if web.wv == nil {
		return
	}
	page := web.wv.Page()
	w.navButton(c, lk, iconChevronLeft, locales.WebBack(), page.CanGoBack(), page.GoBack)
	w.navButton(c, lk, iconChevronRight, locales.WebForward(), page.CanGoForward(), page.GoForward)
	w.navButton(c, lk, iconRefresh, locales.WebReload(), true, func() {
		web.loading, web.err = true, ""
		page.Reload()
	})
	w.rowButton(c, lk, iconExternal, locales.WebOpenInBrowser(), func() { c.OpenURL(web.url) })
}

// navButton is a page action that shows while it can go.
func (w *Window) navButton(c *ui.Context, lk *look, icon *ui.SVG, tip string, enabled bool, fn func()) {
	b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(tip)
	if !enabled {
		b.Disabled(true)
	} else if b.Hovered() {
		b.Background(lk.chip)
	}
	b.Children(func() {
		fg := lk.textMuted
		if !enabled {
			fg = lk.textFaint
		}
		ui.Icon(c, icon).Size(12, 12).TextColor(fg)
	})
	b.Tooltip(tip)
	if enabled && b.Clicked() {
		fn()
	}
}

// webBody is a page in the pane, or what to do when it could not show.
func (w *Window) webBody(c *ui.Context, lk *look, web *webTab) {
	switch {
	case web.err != "":
		ui.Column(c).Fill().Grow(1).Center().Padding(18).Gap(8).Children(func() {
			ui.Icon(c, iconGlobe).Size(22, 22).TextColor(lk.textFaint)
			ui.Text(c, web.err).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.35).MaxWidth(320)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, locales.FilesRetry()).Clicked() && web.wv != nil {
					web.loading, web.err = true, ""
					web.wv.Page().Reload()
				}
				if ui.Button(c, locales.WebOpenInBrowser()).Clicked() {
					c.OpenURL(web.url)
				}
			})
		})
	case web.wv == nil:
		w.filesMessage(c, lk, locales.FilesLoading(), "")
	default:
		ui.WebView(c, web.wv).Fill().Grow(1)
	}
}

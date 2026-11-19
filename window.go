package main

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

const APP_NAME = "Go Shell"

// Window holds the state of the app: the sessions and shortcut commands it
// saved, the open terminal tabs, and the state of its dialogs.
type Window struct {
	ready   chan struct{} // closed once win is set; callbacks wait for it
	win     *mygo.Window
	path    string        // where load and save keep the app's JSON
	repaint chan struct{} // coalesces background repaint requests

	confs    []Config
	cmds     []*Cmd
	tabs     []*Tab
	selected int

	sidebar     float32 // the width of the sidebar, as a percent of the split
	showSidebar bool    // the sidebar of sessions shows
	lastTitle   string  // the window title last shown

	// the right sidebar of files
	showFiles  bool
	filesWidth float32
	filesDrag  float32 // the width as a drag of its edge began, 0 while none
	followDir  bool    // the file sidebar follows the terminal's directory

	// dockerDefaultAdded says the app has looked for a local Docker daemon
	// and added a config for it, or one was already there: it is kept so a
	// config the user deletes does not come back.
	dockerDefaultAdded bool

	// dialogs
	aboutOpen bool
	errOpen   bool
	errMsg    string

	delOpen  bool
	delTitle string
	delMsg   string
	delFn    func()

	confOpen    bool
	confTitle   string
	confEditing Config // the config a modify dialog edits, nil when creating
	confKind    string // the kind a create dialog builds
	pendingSSH  *SSHConfig
	pendingDock *DockerConfig
	pendingK8S  *K8SConfig

	cmdOpen        bool
	cmdEdit        int // the command a modify dialog edits, -1 when creating
	cmdName        string
	cmdText        string
	cmdIcon        string
	cmdAutoSubmit  bool
	cmdIcons       []string
	collapsedHosts map[string]bool // the hosts whose sessions the sidebar folded
	newSessionOpen bool            // the new session popover shows
	sideQuery      string          // the host list's search

	// find
	findOpen      bool // the find bar shows
	findOn        bool // the selected terminal has a search going
	findTab       *Tab // the terminal the search runs in
	findQuery     string
	findLastQuery string
	findTotal     int
	findSel       int
	findSearching bool

	// theme
	appTheme        string // the appearance: "", "light" or "dark"
	themeOpen       bool
	themeQuery      string
	themeNames      []string
	termThemeName   string
	termTheme       *terminal.Theme
	appearanceItems map[string]*mygo.MenuItem

	toast *ui.Toast

	docker *dockerDialog
	k8s    *k8sDialog
}

func newWindow() *Window {
	w := &Window{
		ready:           make(chan struct{}),
		sidebar:         22,
		showSidebar:     true,
		showFiles:       true,
		filesWidth:      280,
		followDir:       true,
		cmdEdit:         -1,
		cmdIcons:        []string{"terminal", "server", "docker", "kubernetes", "cloud", "box", "folder", "file", "document", "computer", "key", "zap", "search", "refresh", "edit", "info"},
		collapsedHosts:  map[string]bool{},
		appearanceItems: map[string]*mygo.MenuItem{},
		repaint:         make(chan struct{}, 1),
	}
	go func() {
		<-w.ready
		for range w.repaint {
			w.win.Update(func() {})
		}
	}()
	return w
}

// invalidate asks for a frame, coalescing a burst of program status reports
// into one repaint.
func (w *Window) invalidate() {
	select {
	case w.repaint <- struct{}{}:
	default:
	}
}

// onMain runs fn on the main thread, where the view builds. Callbacks of
// terminals run on goroutines of their own, and may start before the window
// exists: they wait for it.
func (w *Window) onMain(fn func()) {
	go func() {
		<-w.ready
		w.win.Update(fn)
	}()
}

func (w *Window) showError(err error) {
	if err == nil {
		return
	}
	w.onMain(func() {
		w.errMsg = err.Error()
		w.errOpen = true
	})
}

// showToast shows a message near the bottom of the window for a while.
func (w *Window) showToast(title, typ string) {
	w.onMain(func() {
		w.toast = &ui.Toast{Title: title, Type: typ, Timeout: 3 * time.Second}
	})
}

// view builds the user interface of the window, for every frame.
func (w *Window) view(c *ui.Context) {
	if w.toast != nil {
		c.AddToast(*w.toast)
		w.toast = nil
	}
	w.applyTheme(c)
	lk := lookFor(c)

	ui.Column(c).Fill().Background(lk.wash).Children(func() {
		w.titleBar(c)
		if w.showSidebar {
			ui.SplitPercent(c, &w.sidebar,
				func() { w.sidebarView(c) },
				func() { w.contentView(c) }).Fill().Grow(1)
		} else {
			w.contentView(c)
		}
	})

	w.dialogs(c)
}

// titleBar draws the whole title bar of the frameless window: the window
// controls, the tabs of the open terminals, and the actions as icons, over
// a bar that drags the window (a double click maximizes it). It is 36 DIPs
// low.
func (w *Window) titleBar(c *ui.Context) {
	lk := lookFor(c)
	const h = 36
	mac := runtime.GOOS == "darwin"

	ui.Row(c).FillWidth().Height(h).AlignItems(ui.Center).Gap(4).PaddingStart(12).PaddingEnd(6).Children(func() {
		if mac {
			// traffic lights: close, minimize, zoom, inset from the corner
			// as macOS puts its own
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				w.lightButton(c, "#ff5f57", locales.WinClose(), func() { w.win.Close() })
				w.lightButton(c, "#febc2e", locales.WinMinimize(), func() { w.win.Minimize() })
				w.lightButton(c, "#28c840", locales.WinMaximize(), func() { w.win.ToggleMaximize() })
			})
			ui.Box(c).Width(6)
		}

		w.barButton(c, iconPanel, locales.TooltipToggleSidebar(), w.toggleSidebar)
		w.newSessionButton(c)

		// the room that drags the window, saying which session shows
		ui.Row(c).Grow(1).FillHeight().DragWindow().Children(func() {
			ui.Column(c).Fill().Center().Children(func() {
				title := APP_NAME
				if tab := w.selectedTab(); tab != nil {
					title = tab.Name()
					if tab.cfg != nil {
						title = tab.cfg.Name() + " · " + title
					}
				}
				ui.Text(c, title).FontSize(12.5).FontWeight(500).TextColor(lk.textFaint).
					SingleLine().Ellipsis("…").MaxWidth(420)
			})
		})

		w.barButton(c, iconSearch, locales.MenuFind(), w.toggleFind)
		w.barButton(c, iconPanelRight, locales.TooltipToggleFiles(), w.toggleFiles)

		if !mac {
			w.winButtons(c)
		}
	})
}

// lightButton is a traffic light of macOS: a round, colored button.
func (w *Window) lightButton(c *ui.Context, color, tip string, fn func()) {
	b := ui.ButtonBase(c).Size(12, 12).Radius(6).Background(ui.Hex(color))
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

// winButtons are the window controls of Windows and Linux, at the right
// end of the title bar.
func (w *Window) winButtons(c *ui.Context) {
	lk := lookFor(c)
	w.winButton(c, iconMinimize, locales.WinMinimize(), func() { w.win.Minimize() })
	w.winButton(c, iconMaximize, locales.WinMaximize(), func() { w.win.ToggleMaximize() })

	// close turns red while hovered, as Windows' does
	b := ui.ButtonBase(c).FillHeight().Padding(0, 10).Label(locales.WinClose())
	if b.Hovered() {
		b.Background(lk.danger)
	}
	b.Children(func() {
		ic := ui.Icon(c, iconClose)
		if b.Hovered() {
			ic.TextColor(ui.Hex("#ffffff"))
		}
	})
	b.Tooltip(locales.WinClose())
	if b.Clicked() {
		w.win.Close()
	}
}

func (w *Window) winButton(c *ui.Context, icon *ui.SVG, tip string, fn func()) {
	lk := lookFor(c)
	b := ui.ButtonBase(c).FillHeight().Padding(0, 10).Label(tip)
	if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(14, 14)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

// toggleSidebar shows and hides the sidebar of sessions, as the title
// bar's button and the View menu's Toggle Sidebar do.
func (w *Window) toggleSidebar() {
	w.showSidebar = !w.showSidebar
	w.save()
}

// toggleFiles shows and hides the right sidebar of files.
func (w *Window) toggleFiles() {
	w.showFiles = !w.showFiles
	w.save()
}

// newSessionButton opens a session: on a host chosen from its menu, or
// directly on this machine when there are no hosts to choose from.
func (w *Window) newSessionButton(c *ui.Context) {
	lk := lookFor(c)
	b := ui.ButtonBase(c).Size(32, 26).Radius(6).Center().Gap(2).Label(locales.MenuNewSession())
	if b.Hovered() || w.newSessionOpen {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Icon(c, iconPlus).Size(15, 15).TextColor(lk.textMuted)
		if len(w.confs) > 0 {
			ui.Icon(c, iconChevronDown).Size(8, 8).TextColor(lk.textFaint)
		}
	})
	b.Tooltip(locales.MenuNewSession())
	if b.Clicked() {
		if len(w.confs) == 0 {
			w.startSession(nil)
		} else {
			w.newSessionOpen = !w.newSessionOpen
		}
	}
	if len(w.confs) == 0 {
		return
	}
	ui.Popover(c, b, &w.newSessionOpen, func() {
		ui.Column(c).Width(300).Children(func() {
			ui.Text(c, strings.ToUpper(locales.MenuNewSession())).FontSize(10.5).FontWeight(650).
				LetterSpacing(0.4).TextColor(lk.textFaint).Padding(2, 8, 6, 8)
			ui.Scroll(c).MaxHeight(320).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					w.newSessionRow(c, lk, nil)
					for _, conf := range w.confs {
						w.newSessionRow(c, lk, conf)
					}
				})
			})
			ui.Divider(c).Margin(4, 4, 4, 4)
			row := ui.Row(c).FillWidth().Height(36).Padding(0, 8).Gap(8).Radius(6).AlignItems(ui.Center)
			if row.Hovered() {
				row.Background(lk.rowHover)
			}
			row.Children(func() {
				ui.Box(c).Size(22, 22).Radius(6).Center().Background(lk.chip).Children(func() {
					ui.Icon(c, iconPlus).Size(12, 12).TextColor(lk.textMuted)
				})
				ui.Text(c, locales.MenuNewConfig()).FontSize(12.5).TextColor(lk.text).Grow(1).SingleLine()
			})
			if row.Clicked() {
				w.newSessionOpen = false
				w.openCreate()
			}
		})
	})
}

// newSessionRow is a host in the new session popover.
func (w *Window) newSessionRow(c *ui.Context, lk *look, conf Config) {
	local := conf == nil
	kind, name, detail := "local", locales.LocalHostName(), locales.LocalHostDetail()
	tabs := w.localTabs()
	if !local {
		kind, name, detail = conf.Type(), conf.Name(), confDetail(conf)
		tabs = w.confTabs(conf)
	}
	row := ui.Row(c).FillWidth().Height(40).Padding(0, 8).Gap(8).Radius(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(lk.rowHover)
	}
	tip := name
	if detail != "" {
		tip += " · " + detail
	}
	row.Tooltip(tip)
	row.Children(func() {
		fg, bg := typeTint(kind, lk)
		ui.Box(c).Size(22, 22).Radius(6).Center().Background(bg).Children(func() {
			ui.Icon(c, typeIcon(kind)).Size(12, 12).TextColor(fg)
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(12.5).FontWeight(550).TextColor(lk.text).SingleLine().Ellipsis("…")
			ui.Text(c, detail).FontSize(10.5).TextColor(lk.textMuted).SingleLine().Ellipsis("…")
		})
		if len(tabs) > 0 {
			ui.Textf(c, "%d", len(tabs)).FontSize(10).FontWeight(600).TextColor(lk.textFaint).
				FontFeatures("tnum").Padding(1, 4).Radius(3).Background(lk.chip)
		}
	})
	if row.Clicked() {
		w.newSessionOpen = false
		w.startSession(conf)
	}
}

// barButton is an icon button of the title bar.
func (w *Window) barButton(c *ui.Context, icon *ui.SVG, tip string, fn func()) {
	lk := lookFor(c)
	b := ui.ButtonBase(c).Size(28, 26).Radius(6).Center().Label(tip)
	if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(lk.textMuted)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

// plusButton is the small icon button that adds something, as a shell, a
// config or a command.
func (w *Window) plusButton(c *ui.Context, tip string, fn func()) {
	lk := lookFor(c)
	b := ui.ButtonBase(c).Size(26, 24).Radius(6).Center().Label(tip)
	if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Icon(c, iconPlus).Size(15, 15).TextColor(lk.textMuted)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

func (w *Window) sidebarView(c *ui.Context) {
	lk := lookFor(c)
	// the host list is a card as the file sidebar and the terminal are,
	// inset from the window's left and bottom edges
	ui.Column(c).Fill().Padding(0, 0, 10, 10).Children(func() {
		w.card(c, lk).Children(func() {
			ui.Row(c).FillWidth().Padding(8, 8, 4, 8).Children(func() {
				ui.SearchField(c, &w.sideQuery).FillWidth().Placeholder(locales.SidebarSearch())
			})
			ui.Scroll(c).Fill().Grow(1).Children(func() {
				ui.Column(c).FillWidth().Padding(2, 8, 12, 8).Gap(1).Children(func() {
					q := strings.ToLower(strings.TrimSpace(w.sideQuery))
					shown := false
					local := w.localTabs()
					if q == "" {
						shown = true
						w.hostRow(c, lk, nil, local)
					} else if strings.Contains(strings.ToLower(locales.LocalHostName()), q) ||
						strings.Contains(strings.ToLower(locales.LocalHostDetail()), q) {
						shown = true
						w.hostRow(c, lk, nil, local)
					} else if filtered := w.filterTabs(local, q); len(filtered) > 0 {
						shown = true
						w.hostRow(c, lk, nil, filtered)
					}
					for _, conf := range w.confs {
						tabs := w.confTabs(conf)
						switch {
						case q == "":
						case strings.Contains(strings.ToLower(conf.Name()), q), strings.Contains(strings.ToLower(confDetail(conf)), q):
							// the host itself matches: show all its sessions
						default:
							tabs = w.filterTabs(tabs, q)
							if len(tabs) == 0 {
								continue
							}
						}
						shown = true
						w.hostRow(c, lk, conf, tabs)
					}
					if !shown {
						ui.Text(c, locales.SidebarNoMatch()).FontSize(12).TextColor(lk.textFaint).
							Padding(10, 10).SingleLine()
					}
				})
			})

			w.sidebarFooter(c, lk)
		})
	})
}

// filterTabs keeps the sessions whose names contain q.
func (w *Window) filterTabs(tabs []int, q string) []int {
	out := make([]int, 0, len(tabs))
	for _, i := range tabs {
		if i >= 0 && i < len(w.tabs) && strings.Contains(strings.ToLower(w.tabs[i].Name()), q) {
			out = append(out, i)
		}
	}
	return out
}

// sidebarFooter holds the window's actions at the foot of the host list,
// fixed while the hosts scroll: About, the appearance and the language.
func (w *Window) sidebarFooter(c *ui.Context, lk *look) {
	ui.Row(c).FillWidth().Height(40).Padding(0, 8).Gap(2).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(lk.divider).Children(func() {
		w.barButton(c, iconInfo, locales.MenuAbout(), func() { w.aboutOpen = true })
		if c.Theme().Dark {
			w.barButton(c, iconSun, locales.SwitchToLight(), w.toggleAppearance)
		} else {
			w.barButton(c, iconMoon, locales.SwitchToDark(), w.toggleAppearance)
		}
		w.languageButton(c)
	})
}

// hostRow is one host, local or saved, with its sessions under it. A click
// folds them; a double click starts a new session on the host.
func (w *Window) hostRow(c *ui.Context, lk *look, conf Config, tabs []int) {
	local := conf == nil
	key := hostKey(conf)
	collapsed := w.collapsedHosts[key]
	if w.sideQuery != "" {
		// search results show their sessions, whatever the host's fold state
		collapsed = false
	}
	hasSessions := len(tabs) > 0
	active := false
	for _, i := range tabs {
		if i == w.selected {
			active = true
			break
		}
	}

	name, detail, kind := locales.LocalHostName(), locales.LocalHostDetail(), "local"
	if !local {
		name, detail, kind = conf.Name(), confDetail(conf), conf.Type()
	}
	openState, openText := w.tabsStatus(tabs)

	row := ui.Row(c).FillWidth().Height(42).Padding(0, 8).Gap(8).Radius(8).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	hovered := row.Hovered()
	tip := name
	if detail != "" {
		tip += " · " + detail
	}
	if hasSessions {
		tip += " · " + locales.HostOpenSessions(len(tabs))
	}
	if openText != "" {
		tip += " · " + openText
	}
	row.Tooltip(tip)
	switch {
	case hovered:
		row.Background(lk.rowHover)
	case active:
		// the host of the session on top stays lit
		row.Background(lk.accentSoft)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.MenuNewSession()).Chosen() {
			w.startSession(conf)
		}
		if !local {
			m.Separator()
			if m.Item(locales.TooltipEdit()).Chosen() {
				w.openModify(conf)
			}
			if m.Item(locales.TooltipDelete()).Chosen() {
				w.askDeleteConf(conf)
			}
		}
	}).Children(func() {
		fg, bg := typeTint(kind, lk)
		ui.Box(c).Size(24, 24).Radius(7).Center().Background(bg).Children(func() {
			// the type icon gives way to the fold arrow while the pointer
			// rests on the host
			if hovered && hasSessions && w.sideQuery == "" {
				chev := iconChevronDown
				if collapsed {
					chev = iconChevronRight
				}
				ui.Icon(c, chev).Size(12, 12).TextColor(lk.textMuted)
				return
			}
			ui.Icon(c, typeIcon(kind)).Size(14, 14).TextColor(fg)
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(12.5).FontWeight(550).TextColor(lk.text).
				SingleLine().Ellipsis("…")
			ui.Text(c, detail).FontSize(10.5).TextColor(lk.textMuted).
				SingleLine().Ellipsis("…")
		})
		switch {
		case hovered:
			ui.Row(c).Gap(2).Children(func() {
				w.rowButton(c, lk, iconPlus, locales.MenuNewSession(), func() { w.startSession(conf) })
				if !local {
					w.rowButton(c, lk, iconEdit, locales.TooltipEdit(), func() { w.openModify(conf) })
					w.rowButton(c, lk, iconTrash, locales.TooltipDelete(), func() { w.askDeleteConf(conf) })
				}
			})
		case hasSessions:
			if len(tabs) > 1 {
				ui.Textf(c, "%d", len(tabs)).FontSize(10).FontWeight(600).TextColor(lk.textFaint).
					FontFeatures("tnum").Padding(1, 4).Radius(3).Background(lk.chip)
			}
			tint := lk.accent
			if openState != "" {
				tint, _ = statusTint(lk, openState)
			}
			dot := ui.Box(c).Size(6, 6).Radius(3).Background(tint).Margin(0, 4, 0, 2)
			if openText != "" {
				dot.Tooltip(openText)
			}
		}
	})
	if row.Clicked() && hasSessions && w.sideQuery == "" {
		w.collapsedHosts[key] = !collapsed
	}

	if hasSessions && !collapsed {
		for _, i := range tabs {
			if i >= 0 && i < len(w.tabs) {
				w.sessionRow(c, lk, i, w.tabs[i])
			}
		}
	} else if local && !collapsed && !hasSessions {
		ui.Text(c, locales.LocalHostHint()).FontSize(10.5).TextColor(lk.textFaint).
			Padding(0, 8, 8, 40)
	}
}

// sessionRow is one open terminal under its host.
func (w *Window) sessionRow(c *ui.Context, lk *look, index int, tab *Tab) {
	selected := index == w.selected
	row := ui.Row(c).FillWidth().Height(30).Padding(0, 8, 0, 34).Gap(6).Radius(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	row.BorderWidth(0, 0, 0, 2).BorderColor(ui.Transparent)
	hovered := row.Hovered()
	sum, hasStatus := programRecord{}, false
	if tab.status != nil {
		sum, hasStatus = tab.status.summary()
		hasStatus = hasStatus && sum.state != statusIdle
	}
	switch {
	case selected:
		row.Background(lk.accentSoft)
		row.BorderColor(lk.accent)
	case hovered:
		row.Background(lk.rowHover)
	}
	tip := tab.Name()
	if tab.cfg != nil {
		tip = tab.cfg.Name() + " · " + tip
	}
	if hasStatus {
		tip += " · " + statusText(sum)
	}
	row.Tooltip(tip)
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.TooltipCloseTab()).Chosen() {
			w.removeTab(tab)
		}
	}).Children(func() {
		if hasStatus {
			w.statusMark(c, lk, sum, 12)
		} else {
			ui.Icon(c, iconTerminal).Size(12, 12).TextColor(lk.textFaint)
		}
		fg := lk.textMuted
		if selected {
			fg = lk.text
		}
		ui.Text(c, tab.Name()).FontSize(12.2).TextColor(fg).Grow(1).SingleLine().Ellipsis("…")
		if hovered || selected {
			w.rowButton(c, lk, iconClose, locales.TooltipCloseTab(), func() { w.removeTab(tab) })
		}
	})
	if row.Clicked() {
		w.selected = index
	}
}

// startSession opens a new session on a host, or on this machine when conf
// is nil, and unfolds the host so the new session shows.
func (w *Window) startSession(conf Config) {
	if conf == nil {
		w.collapsedHosts["local"] = false
		w.openLocal()
		return
	}
	w.collapsedHosts[hostKey(conf)] = false
	conf.Term(w)
}

// hostKey identifies a host in the sidebar's folded state.
func hostKey(conf Config) string {
	if conf == nil {
		return "local"
	}
	return fmt.Sprintf("%T:%p", conf, conf)
}

// localTabs are the terminals of this machine, and those of a host that is
// no longer saved.
func (w *Window) localTabs() []int {
	var out []int
	for i, tab := range w.tabs {
		if tab.cfg == nil || !w.hasConf(tab.cfg) {
			out = append(out, i)
		}
	}
	return out
}

func (w *Window) hasConf(conf Config) bool {
	for _, c := range w.confs {
		if c == conf {
			return true
		}
	}
	return false
}

// tabsStatus is the most important state among a set of terminals, "" while
// there are none or they are idle.
func (w *Window) tabsStatus(tabs []int) (string, string) {
	best := -1
	var sum programRecord
	for _, i := range tabs {
		if i < 0 || i >= len(w.tabs) || w.tabs[i].status == nil {
			continue
		}
		s, ok := w.tabs[i].status.summary()
		if !ok || s.state == statusIdle {
			continue
		}
		if rank := statusRank(s.state); rank > best {
			best, sum = rank, s
		}
	}
	if best < 0 {
		return "", ""
	}
	return sum.state, statusText(sum)
}

// confDetail is the line under a session's name: where it connects to.
func confDetail(conf Config) string {
	switch c := conf.(type) {
	case *SSHConfig:
		host := c.data.Host
		if c.data.User != "" && host != "" {
			host = c.data.User + "@" + host
		}
		if host != "" && c.data.Port != 0 && c.data.Port != 22 {
			host += ":" + strconv.Itoa(c.data.Port)
		}
		if host != "" {
			return host
		}
	case *DockerConfig:
		if c.data.Host != "" {
			return c.data.Host
		}
	case *K8SConfig:
		server := strings.TrimPrefix(strings.TrimPrefix(c.data.Server, "https://"), "http://")
		if server != "" {
			return server
		}
	}
	return strings.ToUpper(kindLabel(conf.Type()))
}

// typeTint is a kind's icon color and its soft background.
func typeTint(kind string, lk *look) (ui.Color, ui.Color) {
	if lk.dark {
		switch kind {
		case "ssh":
			return ui.Hex("#b4b4bb"), ui.RGBA(180, 180, 187, 0.14)
		case "docker":
			return ui.Hex("#4db8ff"), ui.RGBA(77, 184, 255, 0.16)
		case "k8s":
			return ui.Hex("#7aa2ff"), ui.RGBA(122, 162, 255, 0.16)
		}
	} else {
		switch kind {
		case "ssh":
			return ui.Hex("#64748b"), ui.RGBA(100, 116, 139, 0.10)
		case "docker":
			return ui.Hex("#2496ed"), ui.RGBA(36, 150, 237, 0.10)
		case "k8s":
			return ui.Hex("#326ce5"), ui.RGBA(50, 108, 229, 0.10)
		}
	}
	return lk.textMuted, lk.chip
}

// confTabs are the terminals a host has open, in tab order.
func (w *Window) confTabs(conf Config) []int {
	var out []int
	for i, tab := range w.tabs {
		if tab.cfg == conf {
			out = append(out, i)
		}
	}
	return out
}

// rowButton is a small action inside a sidebar row.
func (w *Window) rowButton(c *ui.Context, lk *look, icon *ui.SVG, tip string, fn func()) {
	b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(tip)
	if b.Hovered() {
		b.Background(lk.chip)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(12, 12).TextColor(lk.textMuted)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

func (w *Window) contentView(c *ui.Context) {
	lk := lookFor(c)
	if len(w.tabs) == 0 {
		w.syncTitle("")
		ui.Column(c).Fill().MinHeight(0).Padding(0, 10, 10, 6).Children(func() {
			w.card(c, lk).Children(func() {
				ui.Column(c).Fill().Center().Padding(24).Children(func() {
					ui.Box(c).Size(52, 52).Radius(14).Center().Background(lk.chip).Margin(0, 0, 18, 0).Children(func() {
						ui.Icon(c, iconTerminal).Size(26, 26).TextColor(lk.textMuted)
					})
					ui.Text(c, locales.EmptyTerminals()).FontSize(17).FontWeight(650).TextColor(lk.text)
					ui.Text(c, locales.EmptyTerminalsDetail()).FontSize(13).TextColor(lk.textMuted).
						TextAlign(ui.Center).LineHeight(1.4).MaxWidth(420).Margin(6, 0, 20, 0)
					ui.Row(c).Gap(10).Children(func() {
						if ui.PrimaryButton(c, locales.MenuNewShell()).Clicked() {
							w.openLocal()
						}
						if ui.Button(c, locales.MenuNewConfig()).Clicked() {
							w.openCreate()
						}
					})
				})
			})
		})
		return
	}

	tab := w.selectedTab()
	if tab == nil {
		return
	}
	w.syncTitle(tab.Name())

	ui.Row(c).Fill().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		right := float32(10)
		if w.showFiles {
			right = 0
		}
		ui.Column(c).FillHeight().Grow(1).Basis(0).MinWidth(0).Padding(0, right, 10, 6).Children(func() {
			w.card(c, lk).Children(func() {
				w.findBar(c, tab)
				terminal.View(c, tab.term).Fill().Grow(1).AutoFocus()
				w.commandBar(c, lk)
			})
		})
		if w.showFiles {
			w.filesPrepare(tab)
			w.filesHandle(c)
			w.filesPanel(c, lk, tab)
		}
	})
}

// filesHandle is the right sidebar's edge, which a drag moves.
func (w *Window) filesHandle(c *ui.Context) {
	h := ui.Box(c.Key("files-resize")).Width(5).FillHeight().Cursor(ui.CursorResizeColumn)
	if dx, _, ok := h.Dragged(); ok {
		if w.filesDrag == 0 {
			w.filesDrag = w.filesWidth
		}
		w.filesWidth = min(max(w.filesDrag-dx, 200), 560)
	} else if w.filesDrag != 0 && !h.Pressed() {
		w.filesDrag = 0
		w.save()
	}
	if h.DoubleClicked() {
		w.filesWidth = 280
		w.save()
	}
}

// filesPrepare opens a terminal's file source the first time the sidebar
// shows it, and moves it to the terminal's directory while following.
func (w *Window) filesPrepare(tab *Tab) {
	b := tab.files
	if b == nil || b.src == nil {
		return
	}
	if w.followDir {
		if dir := tab.term.Dir(); dir != "" {
			if dir != b.lastTermDir {
				b.lastTermDir = dir
				if b.path != dir {
					w.filesEnter(tab, dir)
				}
			}
			return
		}
		// Docker and SSH's shells do not report OSC 7: ask their sources
		// for the shell's directory now and then instead.
		if src, ok := b.src.(dirSource); ok && !b.polling && time.Since(b.lastPoll) >= followPoll {
			b.lastPoll = time.Now()
			b.polling = true
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				dir, err := src.Dir(ctx)
				w.onMain(func() {
					b.polling = false
					if err != nil || dir == "" || !w.followDir {
						return
					}
					if dir != b.lastTermDir {
						b.lastTermDir = dir
						if b.path != dir {
							w.filesEnter(tab, dir)
						}
					}
				})
			}()
		}
	}
	if b.started {
		return
	}
	b.started = true
	b.loading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		dir, err := b.src.Home(ctx)
		w.onMain(func() {
			b.homeLoaded = true
			if b.path != "" {
				return // follow or the user moved first
			}
			if err != nil || dir == "" {
				b.loading = false
				if err != nil {
					b.err = err.Error()
				} else {
					b.err = locales.FilesUnavailable()
				}
				return
			}
			w.filesEnter(tab, dir)
		})
	}()
}

// filesReload lists the current folder again, or opens the source's home
// when the first listing failed.
func (w *Window) filesReload(tab *Tab) {
	b := tab.files
	if b == nil {
		return
	}
	if b.path == "" {
		b.started, b.homeLoaded = false, false
		b.err = ""
		w.filesPrepare(tab)
		return
	}
	w.filesEnter(tab, b.path)
}

// filesEnter lists a directory in the right sidebar, in the background.
func (w *Window) filesEnter(tab *Tab, dir string) {
	b := tab.files
	if b == nil || b.src == nil || dir == "" {
		return
	}
	b.started = true
	b.path = dir
	b.err = ""
	b.entries = nil
	b.loading = true
	b.request++
	request := b.request
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		entries, err := b.src.List(ctx, dir)
		w.onMain(func() {
			if request != b.request {
				return
			}
			b.loading = false
			if err != nil {
				b.err = err.Error()
				b.entries = nil
				return
			}
			b.entries = entries
		})
	}()
}

// filesPanel is the right sidebar: the folders of the selected terminal.
func (w *Window) filesPanel(c *ui.Context, lk *look, tab *Tab) {
	ui.Column(c).Width(w.filesWidth).Shrink(0).Padding(0, 10, 10, 0).Children(func() {
		w.card(c, lk).Children(func() {
			ui.Row(c).FillWidth().Height(36).Padding(0, 10).Gap(4).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
				ui.Text(c, strings.ToUpper(locales.FilesTitle())).FontSize(11).FontWeight(650).
					LetterSpacing(0.5).TextColor(lk.textFaint).Grow(1).SingleLine()
				w.filesFollowChip(c, lk, tab)
				w.rowButton(c, lk, iconRefresh, locales.FilesRefresh(), func() { w.filesReload(tab) })
				w.rowButton(c, lk, iconClose, locales.FilesClose(), w.toggleFiles)
			})
			w.filesBody(c, lk, tab)
		})
	})
}

// filesFollowChip turns following the terminal's directory on and off.
func (w *Window) filesFollowChip(c *ui.Context, lk *look, tab *Tab) {
	fg := lk.textMuted
	b := ui.ButtonBase(c).Height(22).Padding(0, 7).Radius(5).Gap(4).Label(locales.FilesFollow())
	if w.followDir {
		fg = lk.accent
		b.Background(lk.chip)
	} else if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Icon(c, iconTarget).Size(11, 11).TextColor(fg)
		ui.Text(c, locales.FilesFollow()).FontSize(11).FontWeight(600).TextColor(fg)
	})
	b.Tooltip(locales.FilesFollowTip())
	if b.Clicked() {
		w.followDir = !w.followDir
		if w.followDir && tab.files != nil {
			tab.files.lastTermDir = ""
			tab.files.lastPoll = time.Time{}
		}
		w.save()
	}
}

func (w *Window) filesBody(c *ui.Context, lk *look, tab *Tab) {
	b := tab.files
	if b == nil || b.src == nil {
		w.filesMessage(c, lk, locales.FilesUnavailable(), "")
		return
	}

	parent := b.src.Parent(b.path)
	if b.path != "" {
		ui.Row(c).FillWidth().Height(32).Padding(0, 8).Gap(4).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
			up := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(locales.FilesUp())
			if parent == b.path {
				up.Disabled(true)
			} else {
				if up.Hovered() {
					up.Background(lk.chip)
				}
				up.Children(func() { ui.Icon(c, iconArrowUp).Size(12, 12).TextColor(lk.textMuted) })
				up.Tooltip(locales.FilesUp())
				if up.Clicked() {
					w.filesEnter(tab, parent)
				}
			}
			if parent == b.path {
				up.Children(func() { ui.Icon(c, iconArrowUp).Size(12, 12).TextColor(lk.textFaint) })
			}
			ui.Text(c, b.path).FontSize(12).TextColor(lk.textMuted).Grow(1).SingleLine().Ellipsis("…").Tooltip(b.path)
			if b.loading {
				ui.Spinner(c).Size(11, 11)
			}
		})
	}

	switch {
	case b.loading && len(b.entries) == 0:
		ui.Column(c).Fill().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
	case b.err != "":
		ui.Column(c).Fill().Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, b.err).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).LineHeight(1.35).MaxWidth(220)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				w.filesReload(tab)
			}
		})
	case len(b.entries) == 0:
		w.filesMessage(c, lk, locales.FilesEmpty(), "")
	default:
		ui.Scroll(c).Fill().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(0, 6, 8, 6).Gap(1).Children(func() {
				for _, entry := range b.entries {
					entry := entry
					w.fileRow(c, lk, tab, entry)
				}
			})
		})
	}
}

func (w *Window) filesMessage(c *ui.Context, lk *look, text, detail string) {
	ui.Column(c).Fill().Grow(1).Center().Padding(18).Gap(6).Children(func() {
		ui.Icon(c, iconFolder).Size(22, 22).TextColor(lk.textFaint)
		ui.Text(c, text).FontSize(12.5).TextColor(lk.textMuted).TextAlign(ui.Center).LineHeight(1.35).MaxWidth(220)
		if detail != "" {
			ui.Text(c, detail).FontSize(11.5).TextColor(lk.textFaint).TextAlign(ui.Center).MaxWidth(220)
		}
	})
}

func (w *Window) fileRow(c *ui.Context, lk *look, tab *Tab, entry fileEntry) {
	b := tab.files
	row := ui.Row(c).FillWidth().Height(30).Padding(0, 6).Gap(7).Radius(6).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(lk.rowHover)
		if entry.dir {
			row.Cursor(ui.CursorPointer)
		}
	}
	row.Tooltip(entry.name)
	row.Children(func() {
		fg := lk.textFaint
		icon := iconFile
		if entry.dir {
			icon, fg = iconFolder, lk.textMuted
		}
		ui.Icon(c, icon).Size(13, 13).TextColor(fg)
		ui.Text(c, entry.name).FontSize(12.5).TextColor(lk.text).Grow(1).SingleLine().Ellipsis("…")
		if !entry.dir && entry.sized {
			ui.Text(c, fileSize(entry.size)).FontSize(10.5).TextColor(lk.textFaint).SingleLine()
		}
	})
	if entry.dir && row.Clicked() {
		w.filesEnter(tab, b.src.Join(b.path, entry.name))
	}
}

// card is the surface the terminal and its bars sit on: white in the light,
// a step lighter than the wash in the dark, with a hairline edge and the
// room of the window around it.
func (w *Window) card(c *ui.Context, lk *look) ui.Element {
	card := ui.Column(c).Fill().Grow(1).MinHeight(0).Radius(12).Clip().
		Background(lk.card).Border(1, lk.cardBorder)
	if !lk.dark {
		card.Shadow(0, 1, 3, 0, lk.shadow)
	}
	return card
}

// commandBar holds the shortcut commands under the terminal, as kero holds
// its sessions' actions: quiet chips that stay out of the terminal's way.
func (w *Window) commandBar(c *ui.Context, lk *look) {
	if len(w.cmds) == 0 {
		return
	}
	ui.Row(c).FillWidth().Height(38).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(lk.divider).Children(func() {
		for i, cmd := range w.cmds {
			i, cmd := i, cmd
			b := ui.ButtonBase(c).Height(26).Padding(0, 9).Radius(6).Gap(5).Label(cmd.Name)
			hovered := b.Hovered()
			if hovered {
				b.Background(lk.chip)
			}
			deleted := false
			b.Children(func() {
				if icon := cmdIcon(cmd.Icon); icon != nil {
					ui.Icon(c, icon).Size(13, 13).TextColor(lk.textMuted)
				}
				ui.Text(c, cmd.Name).FontSize(12.5).TextColor(lk.text).SingleLine()
				if hovered {
					// room for the delete badge at the chip's top right
					ui.Box(c).Size(10, 10)
					badge := ui.ButtonBase(c).Absolute().Top(1).Right(2).Size(14, 14).Radius(7).
						Center().Background(lk.danger).Label(locales.TooltipDelete())
					badge.Children(func() {
						ui.Icon(c, iconClose).Size(8, 8).TextColor(ui.Hex("#ffffff"))
					})
					badge.Tooltip(locales.TooltipDelete())
					if badge.Clicked() {
						deleted = true
						w.askDeleteCmd(cmd)
					}
				}
			})
			b.Tooltip(cmd.Text)
			b.ContextMenu(func(m *ui.Menu) {
				if m.Item(locales.TooltipEdit()).Chosen() {
					w.editCmd(i)
				}
				if m.Item(locales.TooltipDelete()).Chosen() {
					w.askDeleteCmd(cmd)
				}
			})
			if b.Clicked() && !deleted {
				w.sendCmd(cmd)
			}
		}
		ui.Spacer(c)
		w.plusButton(c, locales.TooltipNewCommand(), w.openNewCmd)
	})
}

// kindLabel names a kind of host.
func kindLabel(kind string) string {
	switch kind {
	case "ssh":
		return "SSH"
	case "docker":
		return "Docker"
	case "k8s":
		return "Kubernetes"
	}
	return kind
}

// syncTitle shows the tab's name in the window title, after the app's.
func (w *Window) syncTitle(name string) {
	title := APP_NAME
	if name != "" {
		title = APP_NAME + " — " + name
	}
	if title != w.lastTitle {
		w.lastTitle = title
		if w.win != nil {
			w.win.SetTitle(title)
		}
	}
}

func (w *Window) confsOf(kind string) []Config {
	var confs []Config
	for _, conf := range w.confs {
		if conf.Type() == kind {
			confs = append(confs, conf)
		}
	}
	return confs
}

func typeIcon(kind string) *ui.SVG {
	switch kind {
	case "ssh":
		return iconServer
	case "docker":
		return iconDocker
	case "k8s":
		return iconKubernetes
	}
	return iconComputer
}

// removeConf takes a config out of the sidebar.
func (w *Window) removeConf(conf Config) {
	for i, c := range w.confs {
		if c == conf {
			w.confs = append(w.confs[:i], w.confs[i+1:]...)
			break
		}
	}
	w.save()
}

// removeCmd takes a command out of the command bar.
func (w *Window) removeCmd(cmd *Cmd) {
	for i, c := range w.cmds {
		if c == cmd {
			w.cmds = append(w.cmds[:i], w.cmds[i+1:]...)
			break
		}
	}
	w.save()
}

// askDeleteConf asks before a config goes away.
func (w *Window) askDeleteConf(conf Config) {
	w.delTitle = locales.DeleteTitle(conf.Name())
	w.delMsg = locales.DeleteSessionMsg()
	w.delFn = func() { w.removeConf(conf) }
	w.delOpen = true
}

// askDeleteCmd asks before a command goes away.
func (w *Window) askDeleteCmd(cmd *Cmd) {
	w.delTitle = locales.DeleteTitle(cmd.Name)
	w.delMsg = locales.DeleteCommandMsg()
	w.delFn = func() { w.removeCmd(cmd) }
	w.delOpen = true
}

// closeSelected closes the tab the window shows, as the menu's Close Tab does.
func (w *Window) closeSelected() {
	if tab := w.selectedTab(); tab != nil {
		w.removeTab(tab)
	}
}

// toggleFind shows or hides the find bar of the selected terminal, as the
// menu's Find does.
func (w *Window) toggleFind() {
	w.onMain(func() {
		if w.selectedTab() == nil {
			return
		}
		w.findOpen = !w.findOpen
	})
}

// themeLight and themeDark are the themes an appearance chooses from,
// which look.go keeps.

// applyTheme gives the view the colors of the appearance the user chose:
// light, dark, or the system's.
func (w *Window) applyTheme(c *ui.Context) {
	switch w.appTheme {
	case "light":
		c.SetTheme(themeLight)
	case "dark":
		c.SetTheme(themeDark)
	default:
		if mygo.Theme.IsDark() {
			c.SetTheme(themeDark)
		} else {
			c.SetTheme(themeLight)
		}
	}
}

// languageButton switches between the two languages the app ships, showing
// the one a click takes it to.
func (w *Window) languageButton(c *ui.Context) {
	lk := lookFor(c)
	target, label := "zh-Hans", "中文"
	if strings.HasPrefix(i18n.Locale(), "zh") {
		target, label = "en", "EN"
	}
	b := ui.ButtonBase(c).Size(30, 26).Radius(6).Center().Label(label)
	if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(11.5).FontWeight(650).TextColor(lk.textMuted)
	})
	b.Tooltip(label)
	if b.Clicked() {
		mygo.App.SetLocale(target)
	}
}

// isDark reports whether the window shows the dark appearance, as the
// system or the View menu chose.
func (w *Window) isDark() bool {
	switch w.appTheme {
	case "light":
		return false
	case "dark":
		return true
	}
	return mygo.Theme.IsDark()
}

// toggleAppearance switches between the light and the dark appearance, as
// the title bar's button does.
func (w *Window) toggleAppearance() {
	if w.isDark() {
		w.setAppearance("light")
	} else {
		w.setAppearance("dark")
	}
}

// setAppearance remembers the appearance the View menu chose, and updates
// its radio items.
func (w *Window) setAppearance(kind string) {
	if w.appTheme == kind {
		// the menu toggled the item before Click; put it back
		for k, item := range w.appearanceItems {
			item.SetChecked(k == kind)
		}
		return
	}
	w.appTheme = kind
	for k, item := range w.appearanceItems {
		item.SetChecked(k == kind)
	}
	w.save()
}

// findBar shows the find bar of the selected terminal, and runs its search.
func (w *Window) findBar(c *ui.Context, tab *Tab) {
	// a search going on in a tab that is no longer shown ends here
	if w.findTab != nil && w.findTab != tab {
		w.findTab.term.EndSearch()
		w.findTab, w.findOn, w.findLastQuery = nil, false, ""
	}
	if !w.findOpen {
		if w.findOn {
			tab.term.EndSearch()
			w.findOn, w.findTab, w.findLastQuery = false, nil, ""
			w.findTotal, w.findSel, w.findSearching = 0, 0, false
		}
		return
	}
	term := tab.term
	if w.findTab == nil {
		w.findTab, w.findOn = tab, true
		w.findTotal, w.findSel, w.findSearching = 0, 0, false
		// the field still holds a query from the tab before: search here too
		if w.findQuery != "" {
			w.findLastQuery = w.findQuery
			w.findSearching = true
			term.Search(w.findQuery)
		}
	}

	lk := lookFor(c)
	ui.Row(c).FillWidth().Height(40).Padding(0, 10).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
		if ui.SearchField(c, &w.findQuery).Width(240).Changed() {
			if w.findQuery != w.findLastQuery {
				w.findLastQuery = w.findQuery
				w.findTotal, w.findSel, w.findSearching = 0, 0, true
				term.Search(w.findQuery)
			}
		}
		switch {
		case w.findQuery == "":
		case w.findSearching:
			ui.Text(c, locales.FindSearching()).FontSize(12.5).TextColor(lk.textMuted)
		case w.findTotal == 0:
			ui.Text(c, locales.FindNoMatches()).FontSize(12.5).TextColor(lk.textMuted)
		default:
			ui.Textf(c, "%s", locales.FindOf(w.findSel, w.findTotal)).FontSize(12.5).TextColor(lk.textMuted)
		}
		ui.Spacer(c)
		w.rowButton(c, lk, iconChevronUp, locales.TooltipPrevMatch(), term.SearchPrevious)
		w.rowButton(c, lk, iconChevronDown, locales.TooltipNextMatch(), term.SearchNext)
		w.rowButton(c, lk, iconClose, locales.TooltipCloseFind(), func() { w.findOpen = false })
	})
}

// dialogs builds the dialogs of the window, always, for AlertDialog and
// Modal show themselves only while their open flag is set.
func (w *Window) dialogs(c *ui.Context) {
	if w.errOpen {
		ui.AlertDialog(c, &w.errOpen, locales.AlertError(), w.errMsg, locales.BtnOK())
	}
	if w.aboutOpen {
		ui.AlertDialog(c, &w.aboutOpen, APP_NAME, locales.AboutBody(), locales.BtnOK())
	}
	if w.delOpen {
		switch ui.AlertDialog(c, &w.delOpen, w.delTitle, w.delMsg, locales.BtnCancel(), locales.BtnDelete()) {
		case 1:
			w.delFn()
		}
	}
	w.themeDialog(c)
	w.configDialog(c)
	w.cmdDialog(c)
	if w.docker != nil {
		w.docker.dialog(c)
	}
	if w.k8s != nil {
		w.k8s.dialog(c)
	}
}

// statusTint is a state's foreground and soft background.
func statusTint(lk *look, state string) (ui.Color, ui.Color) {
	switch state {
	case statusBlocked:
		return lk.attention, lk.attentionSoft
	case statusDone:
		return lk.success, lk.successSoft
	case statusError:
		return lk.danger, lk.dangerSoft
	default:
		return lk.accent, lk.accentSoft
	}
}

// statusMark is a record's small indicator in the sidebar; its caller
// gives it the record's tooltip.
func (w *Window) statusMark(c *ui.Context, lk *look, rec programRecord, size float32) ui.Element {
	fg, _ := statusTint(lk, rec.state)
	switch rec.state {
	case statusWorking:
		return ui.Spinner(c).Size(size, size).TextColor(fg)
	case statusBlocked, statusError:
		return ui.Icon(c, iconAlert).Size(size, size).TextColor(fg)
	case statusDone:
		return ui.Icon(c, iconCheck).Size(size, size).TextColor(fg)
	}
	return ui.Box(c).Size(size, size)
}

func (w *Window) sendCmd(cmd *Cmd) {
	if tab := w.selectedTab(); tab != nil {
		text := cmd.Text
		if cmd.AutoSubmit {
			text += "\n" // 自动提交：补一个回车
		}
		tab.term.Send([]byte(text))
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

const APP_NAME = "GoShell"

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

	sidebar       float32 // the width of the sidebar, in DIPs
	sidebarResize bool    // the pointer drags the sidebar's edge
	showSidebar   bool    // the sidebar of sessions shows
	lastTitle     string  // the window title last shown

	// the right sidebar of files
	showFiles   bool
	rightView   int // the right sidebar's view: files or git
	filesWidth  float32
	filesResize bool   // the pointer drags the file sidebar's edge
	followDir   bool   // the file sidebar follows the terminal's directory
	openTemp    string // where copies of remote files open, made on first use

	// dockerDefaultAdded says the app has looked for a local Docker daemon
	// and added a config for it, or one was already there: it is kept so a
	// config the user deletes does not come back.
	dockerDefaultAdded bool

	// dialogs
	filePrompt *filePrompt
	confirm    *confirmDialog
	aboutOpen  bool
	errOpen    bool
	errMsg     string

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
	colorTheme      string // the chrome color theme
	colorOpen       bool   // the color theme dialog shows
	themeOpen       bool
	themeQuery      string
	themeNames      []string
	termThemeName   string
	termTheme       *terminal.Theme
	appearanceItems map[string]*mygo.MenuItem

	toast *ui.Toast

	menuSave *mygo.MenuItem

	// the right pane of the middle area: files, pages and commits, side
	// by side with the terminal
	paneTabs  []*paneTab
	paneSel   int
	paneShare float32
	gitShare  float32 // the Git view's share for the changes
	diffSplit bool    // the diffs show side by side, not one over the other

	docker *dockerDialog
	k8s    *k8sDialog
}

func newWindow() *Window {
	w := &Window{
		ready:           make(chan struct{}),
		sidebar:         264,
		showSidebar:     true,
		showFiles:       true,
		colorTheme:      "paper",
		filesWidth:      280,
		followDir:       true,
		paneShare:       defaultPaneShare,
		gitShare:        defaultGitShare,
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
		// a strip of the wash above the cards, as beside and below them
		ui.Row(c).Fill().MinHeight(0).AlignItems(ui.Stretch).Padding(10, 0, 0, 0).Children(func() {
			if w.showSidebar {
				ui.Column(c).FillHeight().Width(w.sidebar).Shrink(0).Children(func() {
					w.sidebarView(c)
				})
				w.sidebarHandle(c, lk)
			}
			ui.Column(c).FillHeight().Grow(1).Basis(0).MinWidth(0).Children(func() {
				w.contentView(c)
			})
		})
	})

	w.dialogs(c)
}

// lightButton is a traffic light of macOS: a round, colored button, at the
// top of the sidebar, or of the middle area when the sidebar is hidden.
func (w *Window) lightButton(c *ui.Context, color, tip string, fn func()) {
	b := ui.ButtonBase(c).Size(12, 12).Radius(6).Background(ui.Hex(color))
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

// winButtons are the window controls of Windows and Linux, at the right
// end of the bar that shows them.
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

// toggleSidebar shows and hides the sidebar of sessions, as its button in
// the terminal's title bar and the View menu's Toggle Sidebar do.
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

// barButton is a quiet icon button of the window's bars.
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

// sidebarHandle is the edge between the host list and the terminal, which
// a drag moves; it draws no line.
func (w *Window) sidebarHandle(c *ui.Context, lk *look) {
	h := ui.Box(c.Key("sidebar-resize")).Width(8).FillHeight().
		Cursor(ui.CursorResizeColumn).KeepFocus()
	if h.Hovered() || h.Pressed() {
		h.Background(lk.divider)
	}
	if dx, _, ok := h.Dragged(); ok {
		// a frame can build twice, so each move arrives once with its
		// distance and once with none: add it, never set the width from it
		w.sidebar = min(max(w.sidebar+dx, 200), 520)
		w.sidebarResize = true
	} else if w.sidebarResize && !h.Pressed() {
		w.sidebarResize = false
		w.save()
	}
	if h.DoubleClicked() {
		w.sidebar = 264
		w.save()
	}
}

func (w *Window) sidebarView(c *ui.Context) {
	lk := lookFor(c)
	// the host list is a card as the file sidebar and the terminal are,
	// inset from the window's left and bottom edges
	ui.Column(c).Fill().Padding(0, 0, 10, 10).Children(func() {
		card := w.card(c, lk)
		// no border on the side facing the terminal: no divider line
		card.BorderWidth(1, 0, 1, 1)
		card.Children(func() {
			w.sidebarHeader(c, lk)
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

// sidebarHeader is the top of the session list: the window's controls, the
// room that drags the window, and the buttons that start a session or add
// a host.
func (w *Window) sidebarHeader(c *ui.Context, lk *look) {
	mac := runtime.GOOS == "darwin"
	ui.Row(c).FillWidth().Height(36).Shrink(0).PaddingStart(12).PaddingEnd(6).Gap(6).
		AlignItems(ui.Center).Children(func() {
		if mac {
			w.windowControls(c)
			ui.Box(c).Width(2)
		}
		ui.Box(c).Grow(1).FillHeight().DragWindow()
		w.newSessionButton(c)
		w.newHostButton(c)
		if !mac {
			w.windowControls(c)
		}
	})
}

// windowControls draws the window's controls, for a window with no title
// bar of its own: the traffic lights on macOS, and minimize, maximize and
// close elsewhere.
func (w *Window) windowControls(c *ui.Context) {
	if runtime.GOOS == "darwin" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			w.lightButton(c, "#ff5f57", locales.WinClose(), func() { w.win.Close() })
			w.lightButton(c, "#febc2e", locales.WinMinimize(), func() { w.win.Minimize() })
			w.lightButton(c, "#28c840", locales.WinMaximize(), func() { w.win.ToggleMaximize() })
		})
		return
	}
	w.winButtons(c)
}

// newHostButton opens the dialog that adds a host.
func (w *Window) newHostButton(c *ui.Context) {
	lk := lookFor(c)
	b := ui.ButtonBase(c).Size(30, 26).Radius(6).Center().Gap(2).Label(locales.MenuNewConfig())
	if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Icon(c, iconServer).Size(14, 14).TextColor(lk.textMuted)
		ui.Icon(c, iconPlus).Size(7, 7).TextColor(lk.textFaint)
	})
	b.Tooltip(locales.MenuNewConfig())
	if b.Clicked() {
		w.openCreate()
	}
}

func (w *Window) sidebarFooter(c *ui.Context, lk *look) {
	ui.Row(c).FillWidth().Height(40).Padding(0, 8).Gap(2).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(lk.divider).Children(func() {
		if v := availableUpdate(); v != "" {
			b := ui.ButtonBase(c).Size(26, 26).Radius(6).Center().Label(locales.UpdateAvailable(v))
			if b.Hovered() {
				b.Background(lk.accentSoft)
			}
			b.Children(func() {
				ui.Icon(c, iconDownload).Size(14, 14).TextColor(lk.accent)
			})
			b.Tooltip(locales.UpdateAvailable(v))
			if b.Clicked() {
				updater.CheckForUpdates()
			}
		}
		w.barButton(c, iconInfo, locales.MenuAbout(), func() { w.aboutOpen = true })
		w.barButton(c, iconPalette, locales.ColorThemeButton(), func() { w.colorOpen = true })
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
	row := ui.Row(c).FillWidth().Height(30).Padding(0, 8).Gap(8).Radius(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	hovered := row.Hovered()
	sum, hasStatus := programRecord{}, false
	if tab.status != nil {
		sum, hasStatus = tab.status.summary()
		hasStatus = hasStatus && sum.state != statusIdle
	}
	switch {
	case selected:
		row.Background(lk.accentSoft)
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
		ui.Box(c).Size(24, 24).Center().Children(func() {
			if hasStatus {
				w.statusMark(c, lk, sum, 13)
			} else {
				ui.Icon(c, iconTerminal).Size(13, 13).TextColor(lk.textFaint)
			}
		})
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
		w.syncMenus()
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
		if w.findOpen || w.findTab != nil {
			w.stopFind()
		}
		ui.Column(c).Fill().MinHeight(0).Padding(0, 10, 10, 6).Children(func() {
			card := w.card(c, lk)
			if w.showSidebar {
				// the edge beside the host list keeps no line
				card.BorderWidth(1, 1, 1, 0)
			}
			card.Children(func() {
				w.emptyHeader(c, lk)
				ui.Column(c).FillWidth().Grow(1).Center().Padding(24).Children(func() {
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
		// the handle between the cards is the gap they show, so the
		// terminal's card keeps no padding on a side a sidebar faces
		left, right := float32(10), float32(10)
		if w.showSidebar {
			left = 0
		}
		if w.showFiles {
			right = 0
		}
		if len(w.paneTabs) == 0 {
			w.terminalView(c, lk, tab, left, right)
		} else {
			// the middle area splits into the terminal and the pane
			w.terminalSplitView(c, lk, tab, left)
			w.paneHandle(c, lk)
			w.paneView(c, lk)
		}
		if w.showFiles {
			w.filesPrepare(tab)
			w.filesHandle(c, lk)
			w.filesPanel(c, lk, tab)
		}
	})
}

// terminalView is the session's terminal, with its command bar, filling
// the middle area.
func (w *Window) terminalView(c *ui.Context, lk *look, tab *Tab, left, right float32) {
	ui.Column(c).FillHeight().Grow(1).Basis(0).MinWidth(0).Padding(0, right, 10, left).Children(func() {
		w.terminalCard(c, lk, tab)
	})
}

// terminalSplitView is the session's terminal on the left of the split
// middle area.
func (w *Window) terminalSplitView(c *ui.Context, lk *look, tab *Tab, left float32) {
	ui.Column(c).FillHeight().Grow(1-w.paneShare).Basis(0).MinWidth(0).
		Padding(0, 0, 10, left).Children(func() {
		w.terminalCard(c, lk, tab)
	})
}

// terminalCard is the terminal and its command bar on the card.
func (w *Window) terminalCard(c *ui.Context, lk *look, tab *Tab) {
	card := w.card(c, lk)
	if w.showSidebar {
		// no line between the host list and the terminal
		card.BorderWidth(1, 1, 1, 0)
	}
	card.Children(func() {
		w.terminalHeader(c, lk, tab)
		terminal.View(c, tab.term).Fill().Grow(1).AutoFocus()
		w.commandBar(c, lk)
	})
}

// emptyHeader is the title bar of the card with no session: the window's
// name, and the sidebars' buttons.
func (w *Window) emptyHeader(c *ui.Context, lk *look) {
	mac := runtime.GOOS == "darwin"
	ui.Row(c).FillWidth().Height(36).Shrink(0).Padding(0, 8).Gap(4).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
		if !w.showSidebar {
			if mac {
				ui.Row(c).PaddingStart(4).Children(func() { w.windowControls(c) })
				ui.Box(c).Width(2)
			}
		}
		w.rowButton(c, lk, iconPanel, locales.TooltipToggleSidebar(), w.toggleSidebar)
		ui.Text(c, APP_NAME).FontSize(12.5).FontWeight(500).TextColor(lk.textFaint).
			Grow(1).SingleLine().DragWindow()
		w.rowButton(c, lk, iconPanelRight, locales.TooltipToggleFiles(), w.toggleFiles)
		if !w.showSidebar && !mac {
			w.windowControls(c)
		}
	})
}

// terminalHeader is the terminal's title bar: the sidebars' buttons, the
// session it shows, and its search, or the search's own field and buttons
// while it is open.
func (w *Window) terminalHeader(c *ui.Context, lk *look, tab *Tab) {
	ui.Row(c).FillWidth().Height(36).Shrink(0).Padding(0, 8).Gap(4).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
		if !w.showSidebar && runtime.GOOS == "darwin" {
			// the inset the sidebar header gives them, less this bar's
			ui.Row(c).PaddingStart(4).Children(func() { w.windowControls(c) })
			ui.Box(c).Width(2)
		}
		w.rowButton(c, lk, iconPanel, locales.TooltipToggleSidebar(), w.toggleSidebar)
		if w.findOpen {
			w.findBar(c, lk, tab)
		} else {
			if tab.cfg != nil {
				ui.Text(c, tab.cfg.Name()).FontSize(12).TextColor(lk.textFaint).SingleLine()
				ui.Text(c, "·").FontSize(12).TextColor(lk.textFaint)
			}
			ui.Text(c, tab.Name()).FontSize(12.5).FontWeight(500).TextColor(lk.textMuted).
				Grow(1).SingleLine().Ellipsis("…").DragWindow()
			w.rowButton(c, lk, iconSearch, locales.MenuFind(), w.toggleFind)
			if len(w.paneTabs) == 0 {
				w.rowButton(c, lk, iconPanelRight, locales.TooltipToggleFiles(), w.toggleFiles)
			}
		}
		if !w.showSidebar && runtime.GOOS != "darwin" {
			w.windowControls(c)
		}
	})
}

// filesHandle is the right sidebar's edge, which a drag moves.
func (w *Window) filesHandle(c *ui.Context, lk *look) {
	h := ui.Box(c.Key("files-resize")).Width(8).FillHeight().
		Cursor(ui.CursorResizeColumn).KeepFocus()
	if h.Hovered() || h.Pressed() {
		h.Background(lk.divider)
	}
	if dx, _, ok := h.Dragged(); ok {
		w.filesWidth = min(max(w.filesWidth-dx, 200), 560)
		w.filesResize = true
	} else if w.filesResize && !h.Pressed() {
		w.filesResize = false
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

// The right sidebar's views.
const (
	rightFiles = iota
	rightGit
)

// filesPanel is the right sidebar: the folders of the selected terminal,
// or its Git status.
func (w *Window) filesPanel(c *ui.Context, lk *look, tab *Tab) {
	ui.Column(c).Width(w.filesWidth).Shrink(0).Padding(0, 10, 10, 0).Children(func() {
		w.card(c, lk).Children(func() {
			// a narrow sidebar gives its buttons less room between them
			pad, gap := float32(8), float32(4)
			if w.filesWidth < 260 {
				pad, gap = 6, 2
			}
			ui.Row(c).FillWidth().Height(36).Padding(0, pad).Gap(gap).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(lk.divider).Children(func() {
				seg := ui.Segmented(c, &w.rightView, locales.FilesTitle(), locales.SidebarGit()).
					Height(24).Label(locales.FilesTitle())
				if seg.Changed() {
					w.save()
				}
				ui.Spacer(c)
				if w.rightView == rightGit {
					if b := tab.files; b != nil && b.src != nil && b.path != "" {
						if gr, ok := b.src.(gitRunner); ok {
							w.rowButton(c, lk, iconGraph, locales.GitGraph(), func() {
								w.openGraph(b.src, gr, b.path)
							})
						}
					}
					w.rowButton(c, lk, iconRefresh, locales.FilesRefresh(), func() { w.gitReload(tab) })
				} else {
					w.filesNewButton(c, lk, tab)
					w.filesFollowChip(c, lk, tab)
					w.rowButton(c, lk, iconRefresh, locales.FilesRefresh(), func() { w.filesReload(tab) })
				}
				w.rowButton(c, lk, iconClose, locales.FilesClose(), w.toggleFiles)
			})
			if w.rightView == rightGit {
				w.gitBody(c, lk, tab)
			} else {
				w.filesBody(c, lk, tab)
			}
		})
	})
}

// filesFollowChip turns following the terminal's directory on and off.
func (w *Window) filesFollowChip(c *ui.Context, lk *look, tab *Tab) {
	// a narrow sidebar shows the chip as its icon alone
	if w.filesWidth < 260 {
		b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(locales.FilesFollow())
		fg := lk.textMuted
		if w.followDir {
			fg = lk.accent
			b.Background(lk.chip)
		} else if b.Hovered() {
			b.Background(lk.rowHover)
		}
		b.Children(func() {
			ui.Icon(c, iconTarget).Size(12, 12).TextColor(fg)
		})
		b.Tooltip(locales.FilesFollowTip())
		if b.Clicked() {
			w.toggleFollow(tab)
		}
		return
	}
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
		w.toggleFollow(tab)
	}
}

// toggleFollow turns following the terminal's directory on and off.
func (w *Window) toggleFollow(tab *Tab) {
	w.followDir = !w.followDir
	if w.followDir && tab.files != nil {
		tab.files.lastTermDir = ""
		tab.files.lastPoll = time.Time{}
	}
	w.save()
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
		ui.Scroll(c).Fill().Grow(1).ContextMenu(func(m *ui.Menu) {
			w.filesNewItems(m, tab)
			if m.Item(locales.FilesRefresh()).Chosen() {
				w.filesReload(tab)
			}
			if m.Item(locales.FilesCopyPath()).Chosen() {
				c.WriteClipboard(b.path)
			}
		}).Children(func() {
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
	full := b.src.Join(b.path, entry.name)
	row := ui.Row(c).FillWidth().Height(30).Padding(0, 6).Gap(7).Radius(6).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(lk.rowHover)
		row.Cursor(ui.CursorPointer)
	}
	if w.paneEditor(b.src, full) != nil {
		row.Background(lk.accentSoft)
	}
	row.Tooltip(entry.name)
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.FilesOpen()).Chosen() {
			if entry.dir {
				w.filesEnter(tab, full)
			} else {
				w.filesOpen(tab, entry)
			}
		}
		if !entry.dir && m.Item(locales.FilesOpenWith()).Chosen() {
			w.filesOpenExternally(tab, entry)
		}
		m.Separator()
		if filesWriter(tab) != nil {
			if m.Item(locales.FilesRename()).Chosen() {
				w.filesRename(tab, entry)
			}
			if m.Item(locales.FilesDelete()).Chosen() {
				w.askDeleteFile(tab, entry)
			}
			m.Separator()
		}
		if m.Item(locales.FilesCopyPath()).Chosen() {
			c.WriteClipboard(full)
		}
		if m.Item(locales.FilesCopyName()).Chosen() {
			c.WriteClipboard(entry.name)
		}
	})
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
	if row.DoubleClicked() {
		if entry.dir {
			w.filesEnter(tab, full)
		} else {
			w.filesOpen(tab, entry)
		}
	} else if entry.dir && row.Clicked() {
		w.filesEnter(tab, full)
	}
}

// filesOpen opens a file in the pane's editor, beside the terminal.
func (w *Window) filesOpen(tab *Tab, entry fileEntry) {
	b := tab.files
	if entry.dir || b == nil || b.src == nil {
		return
	}
	w.openFileTab(b.src, b.src.Join(b.path, entry.name), entry.name)
}

// filesOpenExternally opens a file of the sidebar with the app the system
// has for it.
func (w *Window) filesOpenExternally(tab *Tab, entry fileEntry) {
	b := tab.files
	if entry.dir || b == nil || b.src == nil {
		return
	}
	if entry.sized && entry.size > openFileMax {
		w.fileError(locales.FilesTooLarge())
		return
	}
	w.openExternally(b.src, b.src.Join(b.path, entry.name))
}

// openExternally opens a file with the app the system has for it: where it
// is when it is this machine's, or a copy of it here for a file on the
// other side of a network.
func (w *Window) openExternally(src fileSource, file string) {
	if local, ok := src.(fileLocal); ok {
		if p, ok := local.LocalPath(file); ok {
			w.openFile(p)
			return
		}
	}
	rd, ok := src.(fileReader)
	if !ok {
		return
	}
	dir, err := w.openCopyDir()
	if err != nil {
		w.fileError(err.Error())
		return
	}
	sub, err := os.MkdirTemp(dir, "open-")
	if err != nil {
		w.fileError(err.Error())
		return
	}
	dst := filepath.Join(sub, path.Base(file))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		if err := copyFile(ctx, rd, file, dst); err != nil {
			w.fileError(err.Error())
			return
		}
		w.onMain(func() {
			if err := mygo.Shell.OpenPath(dst); err != nil {
				w.errMsg, w.errOpen = err.Error(), true
			}
		})
	}()
}

// openFile opens a file of this machine with the app the system has for
// it, without holding up the frame.
func (w *Window) openFile(file string) {
	go func() {
		if err := mygo.Shell.OpenPath(file); err != nil {
			w.fileError(err.Error())
		}
	}()
}

// openCopyDir is the folder copies of remote files open in, made on first
// use, and emptied when the window closes.
func (w *Window) openCopyDir() (string, error) {
	if w.openTemp == "" {
		dir, err := os.MkdirTemp("", "goshell-open-*")
		if err != nil {
			return "", err
		}
		w.openTemp = dir
	}
	return w.openTemp, nil
}

// copyFile writes what a file on the other side of a network holds to dst
// here, up to the most the sidebar copies.
func copyFile(ctx context.Context, rd fileReader, file, dst string) error {
	rc, err := rd.Reader(ctx, file)
	if err != nil {
		return err
	}
	defer rc.Close()
	data, err := readLimited(rc)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

// fileError shows what went wrong with a file, in a dialog.
func (w *Window) fileError(text string) {
	w.onMain(func() {
		w.errMsg, w.errOpen = text, true
	})
}

// filesWriter returns the session's file source when it can change files,
// or nil when the sidebar only browses them.
func filesWriter(tab *Tab) fileWriter {
	if tab == nil || tab.files == nil || tab.files.src == nil {
		return nil
	}
	wr, _ := tab.files.src.(fileWriter)
	return wr
}

// filesNewButton opens the menu that makes a file or a folder in the
// folder the sidebar shows.
func (w *Window) filesNewButton(c *ui.Context, lk *look, tab *Tab) {
	if filesWriter(tab) == nil {
		return
	}
	b := ui.ButtonBase(c).Size(20, 20).Radius(5).Center().Label(locales.FilesNew())
	if b.Hovered() {
		b.Background(lk.chip)
	}
	b.PopoverMenuButton(func(m *ui.Menu) { w.filesNewItems(m, tab) })
	b.Children(func() {
		ui.Icon(c, iconPlus).Size(12, 12).TextColor(lk.textMuted)
	})
	b.Tooltip(locales.FilesNew())
}

// filesNewItems adds making a folder and a file to m.
func (w *Window) filesNewItems(m *ui.Menu, tab *Tab) {
	if filesWriter(tab) == nil {
		return
	}
	if m.Item(locales.FilesNewFolder()).Chosen() {
		w.filesNew(tab, true)
	}
	if m.Item(locales.FilesNewFile()).Chosen() {
		w.filesNew(tab, false)
	}
}

// filesNew asks for the name of a file or folder to make in the folder the
// sidebar shows.
func (w *Window) filesNew(tab *Tab, dir bool) {
	b := tab.files
	wr := filesWriter(tab)
	if b == nil || wr == nil {
		return
	}
	parent := b.path
	title := locales.FilesNewFileTitle()
	if dir {
		title = locales.FilesNewFolderTitle()
	}
	w.filePrompt = &filePrompt{
		title: title,
		label: locales.FilesName(),
		ok: func(name string) error {
			ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
			defer cancel()
			target := b.src.Join(parent, name)
			if dir {
				return wr.Mkdir(ctx, target)
			}
			return wr.Touch(ctx, target)
		},
		done: func() { w.filesReloadHere(tab, b, parent) },
	}
}

// filesRename asks for a new name for a file or folder.
func (w *Window) filesRename(tab *Tab, entry fileEntry) {
	b := tab.files
	wr := filesWriter(tab)
	if b == nil || wr == nil {
		return
	}
	dir, from := b.path, b.src.Join(b.path, entry.name)
	w.filePrompt = &filePrompt{
		title: locales.FilesRenameTitle(entry.name),
		label: locales.FilesName(),
		value: entry.name,
		ok: func(name string) error {
			if name == entry.name {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
			defer cancel()
			return wr.Rename(ctx, from, b.src.Join(dir, name))
		},
		done: func() { w.filesReloadHere(tab, b, dir) },
	}
}

// askDeleteFile asks before a file or folder goes away.
func (w *Window) askDeleteFile(tab *Tab, entry fileEntry) {
	if filesWriter(tab) == nil || tab.files == nil {
		return
	}
	b := tab.files
	w.delTitle = locales.FilesDeleteTitle(entry.name)
	w.delMsg = locales.FilesDeleteMsg()
	w.delFn = func() { w.deleteFile(tab, b, entry) }
	w.delOpen = true
}

// deleteFile removes a file or folder, a folder with what it holds.
func (w *Window) deleteFile(tab *Tab, b *fileBrowser, entry fileEntry) {
	wr := filesWriter(tab)
	if wr == nil {
		return
	}
	dir, target := b.path, b.src.Join(b.path, entry.name)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		err := wr.Remove(ctx, target, entry.dir)
		w.onMain(func() {
			if err != nil {
				w.errMsg, w.errOpen = err.Error(), true
				return
			}
			w.filesReloadHere(tab, b, dir)
		})
	}()
}

// filesReloadHere lists a folder again when the sidebar still shows it, as
// after a change there.
func (w *Window) filesReloadHere(tab *Tab, b *fileBrowser, dir string) {
	if tab.files == b && b.path == dir {
		w.filesReload(tab)
	}
	if tab.git != nil && tab.git.dir == dir {
		w.gitReload(tab)
	}
}

// confirmDialog is a question with two answers: the second runs fn.
type confirmDialog struct {
	title   string
	message string
	ok      string
	fn      func()
}

// confirmDialog draws a question, and runs its answer once chosen.
func (w *Window) confirmDialog(c *ui.Context) {
	p := w.confirm
	if p == nil {
		return
	}
	open := true
	switch ui.AlertDialog(c, &open, p.title, p.message, locales.BtnCancel(), p.ok) {
	case 1:
		w.confirm = nil
		p.fn()
		return
	}
	if !open {
		w.confirm = nil
	}
}

// filePrompt is the dialog that names a file or folder: ok changes the
// files, and done runs after it did, on the window's thread.
type filePrompt struct {
	title string
	label string
	value string
	err   string
	busy  bool
	ok    func(name string) error
	done  func()
}

// validFileName reports whether name can be a file or folder's name.
func validFileName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\")
}

// submitFilePrompt makes or renames the file or folder the dialog names,
// and shows what went wrong where it did.
func (w *Window) submitFilePrompt() {
	p := w.filePrompt
	if p == nil || p.busy {
		return
	}
	name := strings.TrimSpace(p.value)
	if !validFileName(name) {
		p.err = locales.FilesNameInvalid()
		return
	}
	p.err, p.busy = "", true
	op, done := p.ok, p.done
	go func() {
		err := op(name)
		w.onMain(func() {
			p.busy = false
			if err != nil {
				p.err = err.Error()
				return
			}
			if w.filePrompt == p {
				w.filePrompt = nil
			}
			if done != nil {
				done()
			}
		})
	}()
}

// filePromptDialog draws the dialog that names a file or folder.
func (w *Window) filePromptDialog(c *ui.Context) {
	p := w.filePrompt
	if p == nil {
		return
	}
	lk := lookFor(c)
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Padding(20).Gap(14).Children(func() {
			dialogHeading(c, p.title)
			submitted := false
			ui.Form(c, func() {
				ui.Field(c, p.label, func() {
					in := ui.TextInput(c, &p.value).AutoFocus()
					if p.busy {
						in.ReadOnly(true)
					}
					submitted = in.Submitted()
				})
			})
			if p.err != "" {
				ui.Text(c, p.err).FontSize(12.5).TextColor(lk.danger).LineHeight(1.35)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				cancel := ui.Button(c, locales.BtnCancel())
				if p.busy {
					cancel.Disabled(true)
				}
				if cancel.Clicked() {
					open = false
				}
				ok := ui.PrimaryButton(c, locales.BtnOK())
				if p.busy {
					ok.Disabled(true)
				}
				if ok.Clicked() || submitted {
					w.submitFilePrompt()
				}
			})
		})
	})
	if !open {
		w.filePrompt = nil
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
		// a focused editor finds its own text; the terminal finds its own
		if w.editorFind() {
			return
		}
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
func (w *Window) findBar(c *ui.Context, lk *look, tab *Tab) {
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

	ui.Spacer(c)
	if ui.SearchField(c, &w.findQuery).Width(240).Height(24).Changed() {
		if w.findQuery != w.findLastQuery {
			w.findLastQuery = w.findQuery
			w.findTotal, w.findSel, w.findSearching = 0, 0, true
			term.Search(w.findQuery)
		}
	}
	switch {
	case w.findQuery == "":
	case w.findSearching:
		ui.Text(c, locales.FindSearching()).FontSize(11.5).TextColor(lk.textMuted).SingleLine()
	case w.findTotal == 0:
		ui.Text(c, locales.FindNoMatches()).FontSize(11.5).TextColor(lk.textMuted).SingleLine()
	default:
		ui.Textf(c, "%s", locales.FindOf(w.findSel, w.findTotal)).FontSize(11.5).TextColor(lk.textMuted).SingleLine()
	}
	w.rowButton(c, lk, iconChevronUp, locales.TooltipPrevMatch(), term.SearchPrevious)
	w.rowButton(c, lk, iconChevronDown, locales.TooltipNextMatch(), term.SearchNext)
	w.rowButton(c, lk, iconClose, locales.TooltipCloseFind(), func() { w.findOpen = false })
}

// stopFind closes the find bar and ends its search.
func (w *Window) stopFind() {
	if w.findTab != nil {
		w.findTab.term.EndSearch()
	}
	w.findOpen, w.findOn, w.findTab, w.findLastQuery = false, false, nil, ""
	w.findTotal, w.findSel, w.findSearching = 0, 0, false
}

// availableUpdate is the version found by the updater, "" when no updater
// plugin runs, as in a view built by a test.
func availableUpdate() (version string) {
	defer func() { _ = recover() }()
	return updater.AvailableVersion()
}

// colorThemeLabel names a color theme in the app's language.
func colorThemeLabel(id string) string {
	switch id {
	case "ocean":
		return locales.ColorThemeOcean()
	case "forest":
		return locales.ColorThemeForest()
	case "sunset":
		return locales.ColorThemeSunset()
	case "violet":
		return locales.ColorThemeViolet()
	case "mono":
		return locales.ColorThemeMono()
	}
	return locales.ColorThemePaper()
}

// colorThemeDialog lists the app's color themes with a small preview of each.
func (w *Window) colorThemeDialog(c *ui.Context) {
	lk := lookFor(c)
	ui.Modal(c, &w.colorOpen, func() {
		ui.Column(c).Width(500).Padding(20).Gap(12).Children(func() {
			dialogHeading(c, locales.ColorThemeTitle())
			ui.Text(c, locales.ColorThemeHint()).FontSize(12).TextColor(lk.textMuted)
			ui.Scroll(c).MaxHeight(410).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					dark := c.Theme().Dark
					for _, th := range colorThemes {
						th := th
						selected := w.colorTheme == th.id
						row := ui.Row(c).FillWidth().Height(58).Padding(0, 8).Gap(10).Radius(8).
							AlignItems(ui.Center).Cursor(ui.CursorPointer)
						switch {
						case selected:
							row.Background(lk.accentSoft)
						case row.Hovered():
							row.Background(lk.rowHover)
						}
						row.Children(func() {
							preview := th.light
							if dark {
								preview = th.dark
							}
							w.colorThemePreview(c, preview)
							ui.Text(c, colorThemeLabel(th.id)).FontSize(13).FontWeight(550).
								TextColor(lk.text).Grow(1).SingleLine()
							if selected {
								ui.Icon(c, iconCheck).Size(14, 14).TextColor(lk.accent)
							}
						})
						if row.Clicked() {
							w.colorTheme = applyColorTheme(th.id)
							w.save()
						}
					}
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.BtnOK()).Clicked() {
					w.colorOpen = false
				}
			})
		})
	})
}

// colorThemePreview is a small window in a theme's colors: its wash, two
// cards, an accent, and a chosen row.
func (w *Window) colorThemePreview(c *ui.Context, th *look) {
	ui.Box(c).Size(78, 44).Radius(9).Clip().Background(th.wash).
		Border(1, th.cardBorder).Children(func() {
		ui.Box(c).Absolute().Left(6).Top(7).Width(30).Height(30).Radius(5).Background(th.card)
		ui.Box(c).Absolute().Left(9).Top(11).Width(16).Height(4).Radius(2).Background(th.accent)
		ui.Box(c).Absolute().Left(9).Top(19).Width(11).Height(3).Radius(1.5).Background(th.textFaint)
		ui.Box(c).Absolute().Left(6).Top(28).Width(30).Height(9).Radius(3).Background(th.accentSoft)
		ui.Box(c).Absolute().Right(6).Top(7).Width(28).Height(30).Radius(5).Background(th.card)
		ui.Box(c).Absolute().Right(9).Top(11).Width(12).Height(3).Radius(1.5).Background(th.accent)
		ui.Box(c).Absolute().Right(9).Top(17).Width(17).Height(3).Radius(1.5).Background(th.rowHover)
		ui.Box(c).Absolute().Right(9).Top(23).Width(9).Height(3).Radius(1.5).Background(th.textMuted)
	})
}

// aboutDialog tells the running version and offers the update check.
func (w *Window) aboutDialog(c *ui.Context) {
	lk := lookFor(c)
	ui.Modal(c, &w.aboutOpen, func() {
		ui.Column(c).Width(440).Padding(20).Gap(12).Children(func() {
			dialogHeading(c, APP_NAME)
			version := mygo.App.Version()
			if version == "" {
				version = "dev"
			}
			ui.Text(c, locales.AboutVersion(version)).FontSize(12).TextColor(lk.textMuted).SingleLine()
			ui.Text(c, locales.AboutBody()).FontSize(12.5).TextColor(lk.textMuted).LineHeight(1.4)
			if v := availableUpdate(); v != "" {
				ui.Row(c).FillWidth().Height(38).Padding(0, 10).Gap(8).Radius(8).
					Background(lk.accentSoft).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconDownload).Size(14, 14).TextColor(lk.accent)
					ui.Text(c, locales.UpdateAvailable(v)).FontSize(12.5).TextColor(lk.text).
						Grow(1).SingleLine().Ellipsis("…")
					if ui.Button(c, locales.AboutUpdateCheck()).Clicked() {
						w.aboutOpen = false
						updater.CheckForUpdates()
					}
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, locales.AboutUpdateCheck()).Clicked() {
					w.aboutOpen = false
					updater.CheckForUpdates()
				}
				ui.Spacer(c)
				if ui.Button(c, locales.BtnOK()).Clicked() {
					w.aboutOpen = false
				}
			})
		})
	})
}

// dialogs builds the dialogs of the window, always, for AlertDialog and
// Modal show themselves only while their open flag is set.
func (w *Window) dialogs(c *ui.Context) {
	if w.errOpen {
		ui.AlertDialog(c, &w.errOpen, locales.AlertError(), w.errMsg, locales.BtnOK())
	}
	w.aboutDialog(c)
	if w.delOpen {
		switch ui.AlertDialog(c, &w.delOpen, w.delTitle, w.delMsg, locales.BtnCancel(), locales.BtnDelete()) {
		case 1:
			w.delFn()
		}
	}
	w.filePromptDialog(c)
	w.confirmDialog(c)
	w.colorThemeDialog(c)
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

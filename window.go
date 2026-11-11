package main

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

const APP_NAME = "Go Shell"

// Window holds the state of the app: the sessions and shortcut commands it
// saved, the open terminal tabs, and the state of its dialogs.
type Window struct {
	ready chan struct{} // closed once win is set; callbacks wait for it
	win   *mygo.Window
	path  string // where load and save keep the app's JSON

	confs    []Config
	cmds     []*Cmd
	tabs     []*Tab
	selected int

	sidebar     float32 // the width of the sidebar, as a percent of the split
	showSidebar bool    // the sidebar of sessions shows
	lastTitle   string  // the window title last shown

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

	cmdOpen       bool
	cmdEdit       int // the command a modify dialog edits, -1 when creating
	cmdName       string
	cmdText       string
	cmdIcon       string
	cmdAutoSubmit bool
	cmdIcons      []string

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
	return &Window{
		ready:           make(chan struct{}),
		sidebar:         20,
		showSidebar:     true,
		cmdEdit:         -1,
		cmdIcons:        []string{"file", "document", "computer"},
		appearanceItems: map[string]*mygo.MenuItem{},
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

	ui.Column(c).Fill().Children(func() {
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
// a bar that drags the window (a double click maximizes it). It is 34 DIPs
// low.
func (w *Window) titleBar(c *ui.Context) {
	t := c.Theme()
	const h = 34
	mac := runtime.GOOS == "darwin"

	ui.Row(c).FillWidth().Height(h).AlignItems(ui.Center).Gap(4).PaddingStart(12).PaddingEnd(8).Children(func() {
		if mac {
			// traffic lights: close, minimize, zoom, inset from the corner
			// as macOS puts its own
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				w.lightButton(c, "#ff5f57", locales.WinClose(), func() { w.win.Close() })
				w.lightButton(c, "#febc2e", locales.WinMinimize(), func() { w.win.Minimize() })
				w.lightButton(c, "#28c840", locales.WinMaximize(), func() { w.win.ToggleMaximize() })
			})
			ui.Box(c).Width(4)
		}

		w.barButton(c, iconPanel, locales.TooltipToggleSidebar(), w.toggleSidebar)

		// the tabs of the open terminals, then a + that opens a shell
		for i, tab := range w.tabs {
			i, tab := i, tab
			w.titleTab(c, t, i, tab)
		}
		w.plusButton(c, locales.MenuNewShell(), w.openLocal)

		// the room that drags the window, saying the app's name while no
		// tab is open
		ui.Row(c).Grow(1).FillHeight().DragWindow().Children(func() {
			ui.Column(c).Fill().Center().Children(func() {
				if len(w.tabs) == 0 {
					ui.Text(c, APP_NAME).TextColor(t.TextMuted)
				}
			})
		})

		w.barButton(c, iconZap, locales.MenuNewCommand(), w.openNewCmd)
		w.barButton(c, iconSearch, locales.MenuFind(), w.toggleFind)
		w.barButton(c, iconInfo, locales.MenuAbout(), func() { w.aboutOpen = true })

		if !mac {
			w.winButtons(c, t)
		}
	})
}

// titleTab is a tab of the title bar: it chooses itself on a click, and
// closes itself on the ✕ inside it.
func (w *Window) titleTab(c *ui.Context, t *ui.Theme, i int, tab *Tab) {
	selected := i == w.selected
	el := ui.ButtonBase(c).Padding(3, 10).Radius(6).Gap(6)
	if selected || el.Hovered() {
		el.Background(t.SurfaceHover)
	}
	el.Children(func() {
		icon := iconTerminal
		if tab.cfg != nil {
			icon = typeIcon(tab.cfg.Type())
		}
		ui.Icon(c, icon)
		ui.Text(c, tab.Name()).SingleLine().
			TextColor(func() ui.Color {
				if selected {
					return t.Text
				}
				return t.TextMuted
			}())
		close := ui.ButtonBase(c).Padding(2).Radius(4)
		if close.Hovered() {
			close.Background(t.Surface)
		}
		close.Children(func() {
			ui.Icon(c, iconClose)
		})
		close.Tooltip(locales.TooltipCloseTab())
		if close.Clicked() {
			w.removeTab(tab)
		}
	})
	if el.Clicked() {
		w.selected = i
	}
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
func (w *Window) winButtons(c *ui.Context, t *ui.Theme) {
	w.winButton(c, t, iconMinimize, locales.WinMinimize(), func() { w.win.Minimize() })
	w.winButton(c, t, iconMaximize, locales.WinMaximize(), func() { w.win.ToggleMaximize() })

	// close turns red while hovered, as Windows' does
	b := ui.ButtonBase(c).FillHeight().Padding(0, 10)
	if b.Hovered() {
		b.Background(t.Danger)
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

func (w *Window) winButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, tip string, fn func()) {
	b := ui.ButtonBase(c).FillHeight().Padding(0, 10)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, icon)
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

// barButton is an icon button of the title bar.
func (w *Window) barButton(c *ui.Context, icon *ui.SVG, tip string, fn func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Padding(5, 8).Radius(6)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, icon)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

// plusButton is the small icon button that adds something, as a shell, a
// config or a command.
func (w *Window) plusButton(c *ui.Context, tip string, fn func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Padding(5, 6).Radius(6)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, iconPlus)
	})
	b.Tooltip(tip)
	if b.Clicked() {
		fn()
	}
}

func (w *Window) sidebarView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).FillWidth().Padding(12).PaddingY(8).Gap(6).Children(func() {
			ui.Text(c, locales.SidebarTitle()).Grow(1)
			w.plusButton(c, locales.TooltipNewConfig(), w.openCreate)
		})

		ui.Scroll(c).Fill().Grow(1).Children(func() {
			ui.Column(c).FillWidth().PaddingX(8).PaddingY(4).Gap(2).Children(func() {
				for _, kind := range []string{"ssh", "docker", "k8s"} {
					confs := w.confsOf(kind)
					if len(confs) == 0 {
						continue
					}
					ui.Row(c).FillWidth().PaddingX(4).PaddingY(4).Gap(6).Children(func() {
						ui.Icon(c, typeIcon(kind))
						ui.Textf(c, "%s (%d)", kind, len(confs))
					})
					for _, conf := range confs {
						w.confRow(c, conf)
					}
				}
				if len(w.confs) == 0 {
					ui.Column(c).FillWidth().Padding(16).Gap(4).Children(func() {
						ui.Text(c, locales.SidebarEmpty1()).TextColor(t.TextMuted)
						ui.Text(c, locales.SidebarEmpty2()).TextColor(t.TextMuted)
					})
				}
			})
		})
	})
}

func (w *Window) confRow(c *ui.Context, conf Config) {
	ui.Row(c).FillWidth().Padding(4).Gap(2).
		ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.TooltipOpen()).Chosen() {
				conf.Term(w)
			}
			if m.Item(locales.TooltipEdit()).Chosen() {
				w.openModify(conf)
			}
			if m.Item(locales.TooltipDelete()).Chosen() {
				w.askDeleteConf(conf)
			}
		}).
		Children(func() {
			ui.Icon(c, typeIcon(conf.Type()))
			ui.Text(c, conf.Name()).Grow(1).SingleLine()
			if ui.Button(c, "").Children(func() {
				ui.Icon(c, iconTerminal)
			}).Tooltip(locales.TooltipOpen()).Clicked() {
				conf.Term(w)
			}
			if ui.Button(c, "").Children(func() {
				ui.Icon(c, iconEdit)
			}).Tooltip(locales.TooltipEdit()).Clicked() {
				w.openModify(conf)
			}
			if ui.Button(c, "").Children(func() {
				ui.Icon(c, iconTrash)
			}).Tooltip(locales.TooltipDelete()).Clicked() {
				w.askDeleteConf(conf)
			}
		})
}

func (w *Window) contentView(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		if len(w.tabs) == 0 {
			w.syncTitle("")
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Icon(c, iconTerminal)
				ui.Text(c, locales.EmptyTerminals())
				if ui.Button(c, locales.MenuNewShell()).Clicked() {
					w.openLocal()
				}
			})
			return
		}

		tab := w.selectedTab()
		if tab == nil {
			return
		}
		w.syncTitle(tab.Name())

		w.findBar(c, tab)

		terminal.View(c, tab.term).Fill().Grow(1).AutoFocus()

		// the shortcut commands, right-aligned, when there are any
		if len(w.cmds) > 0 {
			ui.Row(c).FillWidth().Padding(6).Gap(6).Children(func() {
				ui.Spacer(c)
				for i, cmd := range w.cmds {
					i, cmd := i, cmd
					if ui.Button(c, "").Children(func() {
						if icon := cmdIcon(cmd.Icon); icon != nil {
							ui.Icon(c, icon)
						}
						ui.Text(c, cmd.Name)
					}).Tooltip(cmd.Text).
						ContextMenu(func(m *ui.Menu) {
							if m.Item(locales.TooltipEdit()).Chosen() {
								w.editCmd(i)
							}
							if m.Item(locales.TooltipDelete()).Chosen() {
								w.askDeleteCmd(cmd)
							}
						}).
						Clicked() {
						w.sendCmd(cmd)
					}
				}
				w.plusButton(c, locales.TooltipNewCommand(), w.openNewCmd)
			})
		}
	})
}

// syncTitle shows the tab's name in the window title, after the app's.
func (w *Window) syncTitle(name string) {
	title := APP_NAME
	if name != "" {
		title = APP_NAME + " — " + name
	}
	if title != w.lastTitle {
		w.lastTitle = title
		w.win.SetTitle(title)
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
		return iconKey
	case "docker":
		return iconBox
	case "k8s":
		return iconCloud
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

// themeLight and themeDark are the themes an appearance chooses from.
var (
	themeLight = ui.LightTheme()
	themeDark  = ui.DarkTheme()
)

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
	t := c.Theme()
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

	ui.Row(c).FillWidth().Padding(4, 8).Gap(6).Children(func() {
		ui.Spacer(c)
		if ui.SearchField(c, &w.findQuery).Width(220).Changed() {
			if w.findQuery != w.findLastQuery {
				w.findLastQuery = w.findQuery
				w.findTotal, w.findSel, w.findSearching = 0, 0, true
				term.Search(w.findQuery)
			}
		}
		switch {
		case w.findQuery == "":
		case w.findSearching:
			ui.Text(c, locales.FindSearching()).TextColor(t.TextMuted)
		case w.findTotal == 0:
			ui.Text(c, locales.FindNoMatches()).TextColor(t.TextMuted)
		default:
			ui.Textf(c, "%s", locales.FindOf(w.findSel, w.findTotal)).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		if ui.Button(c, "").Children(func() {
			ui.Icon(c, iconChevronUp)
		}).Tooltip(locales.TooltipPrevMatch()).Clicked() {
			term.SearchPrevious()
		}
		if ui.Button(c, "").Children(func() {
			ui.Icon(c, iconChevronDown)
		}).Tooltip(locales.TooltipNextMatch()).Clicked() {
			term.SearchNext()
		}
		if ui.Button(c, "").Children(func() {
			ui.Icon(c, iconClose)
		}).Tooltip(locales.TooltipCloseFind()).Clicked() {
			w.findOpen = false
		}
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
	w.configDialog(c)
	w.cmdDialog(c)
	if w.docker != nil {
		w.docker.dialog(c)
	}
	if w.k8s != nil {
		w.k8s.dialog(c)
	}
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

package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"goshell/locales"
)

// Tab is an open terminal tab.
type Tab struct {
	name   string // the name the tab was given when it was created
	title  string // the title the program set, if any
	term   *terminal.Terminal
	cfg    Config
	closer func() // closes the connection the terminal is fed with
}

func (t *Tab) Name() string {
	if t.title != "" {
		return t.title
	}
	return t.name
}

// newTab makes a terminal for a new tab, which the caller adds to the
// window with addTermTab once its connection is set up.
func (w *Window) newTab(name string, cfg Config, closer func(), opts terminal.Options) (*Tab, *terminal.Terminal, error) {
	tab := &Tab{name: name, cfg: cfg, closer: closer}
	opts.OnTitle = func(title string) {
		w.onMain(func() { tab.title = title })
	}
	opts.OnExit = func(int) {
		w.showToast(locales.ToastSessionEnded(name), "")
		w.removeTab(tab)
	}
	opts.OnSearch = func(s terminal.SearchStatus) {
		w.onMain(func() {
			if w.selectedTab() != tab {
				return
			}
			w.findTotal, w.findSel, w.findSearching = s.Total, s.Selected, s.Searching
		})
	}
	opts.Theme, opts.DarkTheme = w.termTheme, nil
	term, err := terminal.New(opts)
	tab.term = term
	return tab, term, err
}

func (w *Window) addTermTab(tab *Tab) {
	w.tabs = append(w.tabs, tab)
	w.selected = len(w.tabs) - 1
}

func (w *Window) selectedTab() *Tab {
	if w.selected < 0 || w.selected >= len(w.tabs) {
		return nil
	}
	return w.tabs[w.selected]
}

// removeTab closes the tab's terminal and its connection, and takes the tab
// out of the window.
func (w *Window) removeTab(tab *Tab) {
	if tab.term != nil {
		tab.term.Close()
	}
	if tab.closer != nil {
		tab.closer()
	}
	w.onMain(func() {
		for i, t := range w.tabs {
			if t == tab {
				w.tabs = append(w.tabs[:i], w.tabs[i+1:]...)
				if w.selected >= len(w.tabs) {
					w.selected = len(w.tabs) - 1
				}
				break
			}
		}
	})
}

func (w *Window) closeAll() {
	tabs := append([]*Tab(nil), w.tabs...)
	for _, tab := range tabs {
		w.removeTab(tab)
	}
}

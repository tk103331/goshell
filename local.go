package main

import "github.com/egoist/mygo/plugins/terminal"

// openLocal opens a tab with the user's shell.
func (w *Window) openLocal() {
	tab, _, err := w.newTab("local", nil, nil, terminal.Options{})
	if err != nil {
		w.showError(err)
		return
	}
	w.onMain(func() {
		w.addTermTab(tab)
	})
}

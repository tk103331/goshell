package main

import "fyne.io/fyne/v2/dialog"

func (w *Window) createLocalTermTab() {
	tab, err := newLocalTerm()
	if err != nil {
		dialog.NewError(err, w.win)
		return
	}
	w.AddTermTab(tab)
}

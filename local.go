package main

func (w *Window) createLocalTermTab() {
	tab := NewLocalTerm()
	w.AddTermTab(tab)
}

package main

import (
	"fyne.io/fyne/v2/container"
	"github.com/fyne-io/terminal"
	"log"
)

type Term struct {
	name  string
	term  *terminal.Terminal
	tab   container.TabItem
	stat  string
	local bool
}

func (t *Term) send(txt string) {
	t.term.Write([]byte(txt))
}

func (t *Term) FocusGained() {
	if t.term != nil {
		t.term.FocusGained()
	}
}

func newLocalTerm() (*Term, error) {
	term := terminal.New()
	go func() {
		err := term.RunLocalShell()
		if err != nil {
			log.Println(err)
			return
		}
	}()
	return &Term{name: "local", term: term, local: true}, nil
}

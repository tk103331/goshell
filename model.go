package main

type Cmd struct {
	Name string
	Text string
	Icon string
}

func newCmd(name, text string) *Cmd {
	return &Cmd{Name: name, Text: text}
}

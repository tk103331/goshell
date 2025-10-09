package main

type Cmd struct {
	Name         string
	Text         string
	Icon         string
	AutoSubmit   bool // 是否自动提交（回车）
}

func newCmd(name, text string) *Cmd {
	return &Cmd{Name: name, Text: text}
}

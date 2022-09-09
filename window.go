package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/fyne-io/terminal"
)

const APP_NAME = "Go Shell"
const APP_KEY = "com.github.tk103331.goshell"
const APP_SESSIONS = "sessions"
const APP_COMMANDS = "commands"

var iconMap map[string]fyne.Resource

func init() {
	iconMap = make(map[string]fyne.Resource)
	iconMap["file"] = theme.FileIcon()
	iconMap["document"] = theme.DocumentIcon()
	iconMap["computer"] = theme.ComputerIcon()
}

type Window struct {
	app   fyne.App
	win   fyne.Window
	tabs  *container.DocTabs
	terms map[*container.TabItem]*Term
	confs []Config
	cmds  []*Cmd

	cmdbar *fyne.Container
}

func (w *Window) AddTermTab(tab *Term) {
	tabItem := container.TabItem{Text: tab.Name(), Icon: theme.ComputerIcon(), Content: tab.term}
	tab.AddConfigListener(func(config *terminal.Config) {
		if len(config.Title) > 0 {
			tabItem.Text = config.Title
		} else {
			tabItem.Text = tab.Name()
		}
	})
	w.tabs.Append(&tabItem)
	w.terms[&tabItem] = tab
	w.tabs.Select(&tabItem)
}

func (w *Window) AddConfig(conf *SSHConfig) {
	w.confs = append(w.confs, conf)
	w.save()
}

func (w *Window) AddCmd(cmd *Cmd) {
	w.cmds = append(w.cmds, cmd)
	w.save()
	icon := iconMap[cmd.Icon]
	w.cmdbar.Add(widget.NewButtonWithIcon(cmd.Text, icon, func() {
		w.sendCmd(cmd)
	}))
}

func (w *Window) RemoveConfig(index int) {
	if index < 0 || index > len(w.confs) {
		return
	}
	w.confs = append(w.confs[:index], w.confs[index+1:]...)
	w.save()
}

func (w *Window) Run(stop <-chan struct{}) {

	w.app = app.NewWithID(APP_KEY)
	w.app.Settings().SetTheme(theme.DarkTheme())

	go func() {
		defer w.app.Quit()
		<-stop
	}()

	w.load()
	w.terms = make(map[*container.TabItem]*Term)
	w.win = w.app.NewWindow(APP_NAME)
	w.win.Resize(fyne.NewSize(800, 600))
	w.initUI()

	w.win.ShowAndRun()
}

func (w *Window) initUI() {
	toolbar := widget.NewToolbar(widget.NewToolbarAction(theme.ComputerIcon(), func() {
		tab := NewLocalTerm()
		w.AddTermTab(tab)
	}), widget.NewToolbarAction(theme.DocumentIcon(), func() {
		w.showCreateConfigDialog()
	}), widget.NewToolbarAction(theme.ContentAddIcon(), func() {
		w.showNewCmdDialog()
	}),
		widget.NewToolbarSpacer(), widget.NewToolbarAction(theme.InfoIcon(), func() {
			w.showAboutDialog()
		}))

	buttons := make([]fyne.CanvasObject, len(w.cmds))
	for i, cmd := range w.cmds {
		if icon, ok := iconMap[cmd.Icon]; ok {
			buttons[i] = widget.NewButtonWithIcon(cmd.Name, icon, func() {
				w.sendCmd(cmd)
			})
		} else {
			buttons[i] = widget.NewButton(cmd.Name, func() {
				w.sendCmd(cmd)
			})
		}
	}
	w.cmdbar = container.NewHBox(buttons...)

	sidebar := widget.NewList(func() int {
		return len(w.confs)
	}, func() fyne.CanvasObject {
		return container.NewHBox(widget.NewLabel(""), layout.NewSpacer(),
			widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), nil),
			widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),
			widget.NewButtonWithIcon("", theme.ComputerIcon(), nil))
	}, func(id widget.ListItemID, object fyne.CanvasObject) {
		box := object.(*fyne.Container)
		label := box.Objects[0].(*widget.Label)
		edit := box.Objects[2].(*widget.Button)
		del := box.Objects[3].(*widget.Button)
		open := box.Objects[4].(*widget.Button)

		conf := w.confs[id]
		label.Text = conf.Name()
		edit.OnTapped = func() {
			w.showModifyConfigDialog(conf)
		}
		del.OnTapped = func() {
			w.RemoveConfig(id)
		}
		open.OnTapped = func() {
			conf.Term(w)
		}
	})

	w.tabs = container.NewDocTabs()
	w.createLocalTermTab()
	w.tabs.OnClosed = func(item *container.TabItem) {
		if term, ok := w.terms[item]; ok {
			term.Exit()
		}
	}
	center := container.NewHSplit(sidebar, w.tabs)
	center.Offset = 0.2

	content := container.NewBorder(toolbar, w.cmdbar, nil, nil, center)

	w.win.SetContent(content)
}

func (w *Window) showAboutDialog() {
	dialog.NewInformation(APP_NAME, "GoShell is a simple terminal GUI client, written in Go,via Fyne. ", w.win).Show()
}

func (w *Window) showError(e error) {
	dialog.ShowError(e, w.win)
}

func (w *Window) sendCmd(cmd *Cmd) {
	tabItem := w.tabs.Selected()
	if tabItem != nil {
		if term, ok := w.terms[tabItem]; ok {
			term.Send(cmd.Text)
		}
	}
}

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

	// 基础图标
	iconMap["file"] = theme.FileIcon()
	iconMap["document"] = theme.DocumentIcon()
	iconMap["computer"] = theme.ComputerIcon()

	// 操作类图标
	iconMap["search"] = theme.SearchIcon()
	iconMap["download"] = theme.DownloadIcon()
	iconMap["upload"] = theme.UploadIcon()
	iconMap["content-add"] = theme.ContentAddIcon()
	iconMap["content-remove"] = theme.ContentRemoveIcon()
	iconMap["content-copy"] = theme.ContentCopyIcon()
	iconMap["content-cut"] = theme.ContentCutIcon()
	iconMap["content-paste"] = theme.ContentPasteIcon()
	iconMap["content-redo"] = theme.ContentRedoIcon()
	iconMap["content-undo"] = theme.ContentUndoIcon()
	iconMap["content-clear"] = theme.ContentClearIcon()

	// 导航类图标
	iconMap["navigate-back"] = theme.NavigateBackIcon()
	iconMap["navigate-next"] = theme.NavigateNextIcon()
	iconMap["home"] = theme.HomeIcon()

	// 媒体类图标
	iconMap["media-play"] = theme.MediaPlayIcon()
	iconMap["media-pause"] = theme.MediaPauseIcon()
	iconMap["media-stop"] = theme.MediaStopIcon()
	iconMap["media-fast-forward"] = theme.MediaFastForwardIcon()
	iconMap["media-record"] = theme.MediaRecordIcon()
	iconMap["media-skip-next"] = theme.MediaSkipNextIcon()
	iconMap["media-skip-previous"] = theme.MediaSkipPreviousIcon()

	// 工具类图标
	iconMap["help"] = theme.HelpIcon()
	iconMap["info"] = theme.InfoIcon()
	iconMap["warning"] = theme.WarningIcon()
	iconMap["error"] = theme.ErrorIcon()
	iconMap["settings"] = theme.SettingsIcon()

	// 文件夹和存储类图标
	iconMap["folder"] = theme.FolderIcon()
	iconMap["folder-open"] = theme.FolderOpenIcon()

	// 其他常用图标
	iconMap["cancel"] = theme.CancelIcon()
	iconMap["check"] = theme.ConfirmIcon()
	iconMap["delete"] = theme.DeleteIcon()
	iconMap["refresh"] = theme.ViewRefreshIcon()
	iconMap["list"] = theme.ListIcon()
	iconMap["calendar"] = theme.CalendarIcon()
	iconMap["volume-up"] = theme.VolumeUpIcon()
	iconMap["volume-down"] = theme.VolumeDownIcon()
	iconMap["volume-mute"] = theme.VolumeMuteIcon()
	iconMap["visibility"] = theme.VisibilityIcon()
	iconMap["visibility-off"] = theme.VisibilityOffIcon()

	// 系统类图标（避免重复）
	iconMap["info"] = theme.InfoIcon()
	iconMap["warning"] = theme.WarningIcon()
	iconMap["error"] = theme.ErrorIcon()
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
	w.cmdbar.Add(widget.NewButtonWithIcon(cmd.Name, icon, func() {
		w.sendCmd(cmd)
	}))
}

func (w *Window) RemoveCmd(index int) {
	if index < 0 || index >= len(w.cmds) {
		return
	}
	w.cmds = append(w.cmds[:index], w.cmds[index+1:]...)
	w.save()
	w.refreshCmdBar()
}

func (w *Window) UpdateCmd(index int, cmd *Cmd) {
	if index < 0 || index >= len(w.cmds) {
		return
	}
	w.cmds[index] = cmd
	w.save()
	w.refreshCmdBar()
}

func (w *Window) refreshCmdBar() {
	// 清空现有命令栏
	w.cmdbar.Objects = nil

	// 重新添加所有命令按钮
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

	// 刷新显示
	w.cmdbar.Refresh()
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

	// 加载用户设置
	settings := w.LoadSettings()
	w.ApplySettings(settings)

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
	}), widget.NewToolbarAction(theme.ListIcon(), func() {
		w.showCmdManagerDialog()
	}),
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.SettingsIcon(), func() {
			w.showSettingsDialog()
		}),
		widget.NewToolbarAction(theme.InfoIcon(), func() {
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
			// 如果启用了自动提交，则发送回车键
			if cmd.AutoSubmit {
				term.Send("\r")
			}
		}
	}
}

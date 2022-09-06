package main

import (
	"context"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/fyne-io/terminal"
)

type DockerConfigData struct {
	Name string
	Type string
	Host string
}

type DockerConfig struct {
	data *DockerConfigData
	onOk func()
}

func (c *DockerConfig) Name() string {
	return c.data.Name
}

func (c *DockerConfig) Type() string {
	return "ssh"
}

func (c *DockerConfig) Data() interface{} {
	return c.data
}

func (c *DockerConfig) Form() *widget.Form {

	nameEntry := widget.NewEntry()
	hostEntry := widget.NewEntry()

	data := c.data
	if data != nil {
		nameEntry.Text = data.Name
		nameEntry.Disable()
		hostEntry.Text = data.Host
	}
	c.onOk = func() {
		if c.data == nil {
			c.data = &DockerConfigData{Type: c.Type()}
		}
		c.data.Name = nameEntry.Text
		c.data.Host = hostEntry.Text
	}
	return widget.NewForm([]*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Host", hostEntry),
	}...)
}

func (c *DockerConfig) OnOk() {
	c.onOk()
}
func (c *DockerConfig) Term(win *Window) {
	opts := make([]client.Opt, 0)
	if len(c.data.Host) > 0 {
		opts = append(opts, client.WithHost(c.data.Host))
	}
	dockerCli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		win.showError(err)
		return
	}
	contList, err := dockerCli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		win.showError(err)
		return
	}
	var dlg dialog.Dialog
	list := widget.NewList(func() int {
		return len(contList)
	}, func() fyne.CanvasObject {
		return container.NewHBox(widget.NewLabel(""), widget.NewButton("Connect", func() {
		}))
	}, func(id widget.ListItemID, object fyne.CanvasObject) {
		box := object.(*fyne.Container)
		cont := contList[id]

		label := box.Objects[0].(*widget.Label)
		btn := box.Objects[1].(*widget.Button)
		label.SetText(cont.ID[:12])
		btn.OnTapped = func() {

			execId, err := dockerCli.ContainerExecCreate(context.Background(), cont.ID, types.ExecConfig{Tty: true, Detach: true, AttachStdin: true, AttachStderr: true, AttachStdout: true, Cmd: []string{"/bin/sh"}})
			if err != nil {
				win.showError(err)
				return
			}

			attach, err := dockerCli.ContainerExecAttach(context.Background(), execId.ID, types.ExecStartCheck{Tty: true, Detach: false})
			if err != nil {
				win.showError(err)
				return
			}
			term := terminal.New()
			go func() {
				defer attach.Close()
				err = term.RunWithConnection(attach.Conn, attach.Reader)
				if err != nil {
					win.showError(err)
					return
				}
			}()

			tab := &Term{name: c.Name(), term: term}
			win.AddTermTab(tab)
			if dlg != nil {
				dlg.Hide()
			}
		}
	})

	list.Resize(fyne.Size{Width: 400, Height: 500})

	dlg = dialog.NewCustom("Select a container", "Cancel", list, win.win)
	dlg.Resize(fyne.Size{Width: 400, Height: 500})
	dlg.Show()
}

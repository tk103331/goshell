package main

import (
	"context"
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type DockerConfigData struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	Host string `json:"host,omitempty"`
}

type DockerConfig struct {
	data *DockerConfigData
	onOk func()
}

func (c *DockerConfig) Name() string {
	return c.data.Name
}

func (c *DockerConfig) Type() string {
	return "docker"
}

func (c *DockerConfig) Load(s string) error {
	data := &DockerConfigData{}

	err := json.Unmarshal([]byte(s), data)
	if err != nil {
		return err
	}
	c.data = data
	return nil
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
	contList, err := dockerCli.ContainerList(context.Background(), containerTypes.ListOptions{})
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

			execId, err := dockerCli.ContainerExecCreate(context.Background(), cont.ID, containerTypes.ExecOptions{Tty: true, Detach: true, AttachStdin: true, AttachStderr: true, AttachStdout: true, Cmd: []string{"/bin/sh"}})
			if err != nil {
				win.showError(err)
				return
			}

			attach, err := dockerCli.ContainerExecAttach(context.Background(), execId.ID, containerTypes.ExecAttachOptions{Tty: true, Detach: false})
			if err != nil {
				win.showError(err)
				return
			}

			term := NewTerm(c.Name(), c)

			go func() {
				defer attach.Close()
				err = term.RunWithConnection(attach.Conn)
				if err != nil {
					win.showError(err)
					return
				}
			}()

			win.AddTermTab(term)
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

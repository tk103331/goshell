package main

import (
	"context"
	"net"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

type DockerConfigData struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	Host string `json:"host,omitempty"`
}

type DockerConfig struct {
	data *DockerConfigData
}

func (c *DockerConfig) Name() string      { return c.data.Name }
func (c *DockerConfig) Type() string      { return "docker" }
func (c *DockerConfig) Data() interface{} { return c.data }
func (c *DockerConfig) Apply()            { c.data.Type = c.Type() }
func (c *DockerConfig) Prefill()          {}

func (c *DockerConfig) Validate() string {
	if c.data.Name == "" {
		return locales.ValNameRequired()
	}
	if c.data.Host == "" {
		return locales.ValHostRequired()
	}
	return ""
}

func (c *DockerConfig) Fields(ctx *ui.Context, editing bool, w *Window) {
	ui.Field(ctx, locales.FieldName(), func() {
		e := ui.TextInput(ctx, &c.data.Name)
		if editing {
			e.ReadOnly(true)
		}
	})
	ui.Field(ctx, locales.FieldHost(), func() { ui.TextInput(ctx, &c.data.Host) })
}

func (c *DockerConfig) Term(w *Window) {
	opts := make([]client.Opt, 0)
	if len(c.data.Host) > 0 {
		opts = append(opts, client.WithHost(c.data.Host))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		w.showError(err)
		return
	}
	st := &dockerDialog{win: w, cfg: c, cli: cli}
	w.docker = st
	go st.load()
}

// dockerDialog is the dialog that lists a Docker daemon's containers and
// connects to one of them.
type dockerDialog struct {
	win    *Window
	open   bool
	cfg    *DockerConfig
	cli    *client.Client
	conts  []container.Summary
	err    error
	loaded bool
}

func (st *dockerDialog) load() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conts, err := st.cli.ContainerList(ctx, container.ListOptions{})
	st.win.onMain(func() {
		st.conts, st.err, st.loaded = conts, err, true
	})
}

func (st *dockerDialog) dialog(c *ui.Context) {
	ui.Modal(c, &st.open, func() {
		ui.Column(c).Width(460).Padding(16).Gap(12).Children(func() {
			ui.Text(c, locales.DockerTitle())
			ui.Scroll(c).Height(360).Children(func() {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					switch {
					case st.err != nil:
						ui.Text(c, st.err.Error()).TextColor(c.Theme().Danger)
					case !st.loaded:
						ui.Row(c).Gap(8).Padding(8).Children(func() {
							ui.Spinner(c)
							ui.Text(c, locales.DockerLoading())
						})
					case len(st.conts) == 0:
						ui.Text(c, locales.DockerNone()).TextColor(c.Theme().TextMuted)
					}
					for i := range st.conts {
						i := i
						cont := st.conts[i]
						ui.Row(c).FillWidth().Gap(8).Children(func() {
							ui.Text(c, cont.ID[:12]).Grow(1).SingleLine()
							if ui.Button(c, locales.BtnConnect()).Clicked() {
								st.connect(cont.ID)
							}
						})
					}
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.BtnCancel()).Clicked() {
					st.open = false
				}
			})
		})
	})
}

func (st *dockerDialog) connect(contID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	execID, err := st.cli.ContainerExecCreate(ctx, contID, container.ExecOptions{
		Tty:          true,
		AttachStdin:  true,
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          []string{"/bin/sh"},
	})
	if err != nil {
		st.win.showError(err)
		return
	}
	attach, err := st.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{Tty: true})
	if err != nil {
		st.win.showError(err)
		return
	}

	tab, _, err := st.win.newTab(st.cfg.Name(), st.cfg, func() { attach.Close() },
		terminal.Options{Conn: &dockerConn{Conn: attach.Conn, cli: st.cli, execID: execID.ID}})
	if err != nil {
		st.win.showError(err)
		attach.Close()
		return
	}

	st.win.onMain(func() {
		st.open = false
		st.win.addTermTab(tab)
	})
	st.win.showToast(locales.ToastConnected(contID[:12]), "success")
}

// dockerConn connects a terminal to a Docker exec stream, and resizes the
// exec when the view resizes the terminal.
type dockerConn struct {
	net.Conn
	cli    *client.Client
	execID string
}

func (c *dockerConn) Resize(cols, rows int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return c.cli.ContainerExecResize(ctx, c.execID, container.ResizeOptions{
		Width:  uint(cols),
		Height: uint(rows),
	})
}

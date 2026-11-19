package main

import (
	"context"
	"fmt"
	"net"
	"strings"
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
	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if len(c.data.Host) > 0 {
		opts = append(opts, client.WithHost(c.data.Host))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		w.showError(err)
		return
	}
	st := &dockerDialog{win: w, cfg: c, cli: cli, open: true}
	w.docker = st
	go st.load()
}

// dockerDialog is the dialog that lists a Docker daemon's containers and
// connects to one of them.
type dockerDialog struct {
	win     *Window
	open    bool
	cfg     *DockerConfig
	cli     *client.Client
	conts   []container.Summary
	err     error
	loaded  bool
	loading bool
	query   string
}

func (st *dockerDialog) load() {
	st.win.onMain(func() { st.loading, st.err = true, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conts, err := st.cli.ContainerList(ctx, container.ListOptions{})
	st.win.onMain(func() {
		st.conts, st.err, st.loaded, st.loading = conts, err, true, false
	})
}

func (st *dockerDialog) filtered() []container.Summary {
	q := strings.ToLower(strings.TrimSpace(st.query))
	if q == "" {
		return st.conts
	}
	out := make([]container.Summary, 0, len(st.conts))
	for _, c := range st.conts {
		name := strings.Join(c.Names, " ")
		if strings.Contains(strings.ToLower(name), q) ||
			strings.Contains(strings.ToLower(c.Image), q) ||
			strings.Contains(strings.ToLower(c.ID), q) {
			out = append(out, c)
		}
	}
	return out
}

func containerName(c container.Summary) string {
	if len(c.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

func (st *dockerDialog) dialog(c *ui.Context) {
	lk := lookFor(c)
	ui.Modal(c, &st.open, func() {
		ui.Column(c).Width(560).Padding(20).Gap(12).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				dialogHeading(c, locales.DockerTitle())
				ui.Textf(c, "%d", len(st.conts)).FontSize(10.5).FontWeight(600).
					TextColor(lk.textFaint).Padding(1, 5).Radius(4).Background(lk.chip)
				ui.Spacer(c)
				if st.loading {
					ui.Spinner(c).Size(13, 13)
				}
			})
			ui.Text(c, st.cfg.data.Host).FontSize(11.5).TextColor(lk.textFaint).SingleLine().Ellipsis("…")
			ui.SearchField(c, &st.query).Placeholder(locales.DockerSearch())
			ui.Text(c, locales.DockerHint()).FontSize(11.5).TextColor(lk.textFaint).SingleLine()

			ui.Scroll(c).Height(380).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					switch {
					case st.err != nil:
						ui.Column(c).FillWidth().Center().Padding(20).Gap(8).Children(func() {
							ui.Text(c, st.err.Error()).FontSize(12).TextColor(lk.danger).
								TextAlign(ui.Center).MaxWidth(420).LineHeight(1.35)
							if ui.Button(c, locales.BtnRetry()).Clicked() {
								go st.load()
							}
						})
					case !st.loaded:
						ui.Column(c).FillWidth().Height(180).Center().Gap(8).Children(func() {
							ui.Spinner(c).Label(locales.DockerLoading())
							ui.Text(c, locales.DockerLoading()).FontSize(12.5).TextColor(lk.textMuted)
						})
					case len(st.conts) == 0:
						ui.Text(c, locales.DockerNone()).FontSize(12.5).TextColor(lk.textMuted).Padding(8, 10)
					case len(st.filtered()) == 0:
						ui.Text(c, locales.DockerNoMatch()).FontSize(12.5).TextColor(lk.textMuted).Padding(8, 10)
					}
					for i := range st.filtered() {
						st.containerRow(c, lk, st.filtered()[i])
					}
				})
			})

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, locales.BtnRefresh()).Clicked() {
					go st.load()
				}
				ui.Spacer(c)
				if ui.Button(c, locales.BtnCancel()).Clicked() {
					st.open = false
				}
			})
		})
	})
}

func (st *dockerDialog) containerRow(c *ui.Context, lk *look, cont container.Summary) {
	name := containerName(cont)
	if name == "" {
		name = cont.ID[:12]
	}
	row := ui.Row(c).FillWidth().Height(46).Padding(0, 8).Gap(10).Radius(8).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(lk.rowHover)
	}
	tip := name
	if cont.Image != "" {
		tip += " · " + cont.Image
	}
	if cont.Status != "" {
		tip += " · " + cont.Status
	}
	row.Tooltip(tip)
	row.Children(func() {
		fg, bg := typeTint("docker", lk)
		ui.Box(c).Size(28, 28).Radius(7).Center().Background(bg).Children(func() {
			ui.Icon(c, iconDocker).Size(16, 16).TextColor(fg)
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(13).FontWeight(550).TextColor(lk.text).SingleLine().Ellipsis("…")
			detail := cont.Image
			if cont.Status != "" {
				if detail != "" {
					detail += " · "
				}
				detail += cont.Status
			}
			ui.Text(c, detail).FontSize(10.5).TextColor(lk.textMuted).SingleLine().Ellipsis("…")
		})
		state := lk.success
		if cont.State != "running" {
			state = lk.textFaint
		}
		ui.Box(c).Size(6, 6).Radius(3).Background(state)
		ui.Text(c, cont.ID[:12]).FontSize(10.5).TextColor(lk.textFaint).FontFeatures("tnum").SingleLine()
		ui.Icon(c, iconChevronRight).Size(12, 12).TextColor(lk.textFaint)
	})
	if row.Clicked() {
		st.connect(cont.ID)
	}
}
func (st *dockerDialog) connect(contID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// the marker lets the file sidebar find this shell's /proc entry
	token := fmt.Sprintf("goshell-%d", time.Now().UnixNano())
	execID, err := st.cli.ContainerExecCreate(ctx, contID, container.ExecOptions{
		Tty:          true,
		AttachStdin:  true,
		AttachStderr: true,
		AttachStdout: true,
		Env:          []string{"GOSHELL_SESSION=" + token},
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
	tab.useFiles(&dockerFiles{cli: st.cli, id: contID, marker: token})

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

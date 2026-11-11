package main

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/tk103331/stream"
	"goshell/locales"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type K8SConfigData struct {
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	Server string `json:"server,omitempty"`
	Token  string `json:"token,omitempty"`
}

type K8SConfig struct {
	data *K8SConfigData
}

func (c *K8SConfig) Name() string      { return c.data.Name }
func (c *K8SConfig) Type() string      { return "k8s" }
func (c *K8SConfig) Data() interface{} { return c.data }
func (c *K8SConfig) Apply()            { c.data.Type = c.Type() }
func (c *K8SConfig) Prefill()          {}

func (c *K8SConfig) Validate() string {
	if c.data.Name == "" {
		return locales.ValNameRequired()
	}
	if c.data.Server == "" {
		return locales.ValServerRequired()
	}
	if c.data.Token == "" {
		return locales.ValTokenRequired()
	}
	return ""
}

func (c *K8SConfig) Fields(ctx *ui.Context, editing bool, w *Window) {
	ui.Field(ctx, locales.FieldName(), func() {
		e := ui.TextInput(ctx, &c.data.Name)
		if editing {
			e.ReadOnly(true)
		}
	})
	ui.Field(ctx, locales.FieldServer(), func() { ui.TextInput(ctx, &c.data.Server) })
	ui.Field(ctx, locales.FieldToken(), func() { ui.TextArea(ctx, &c.data.Token).Lines(3, 6) })
}

func (c *K8SConfig) clientset() (*kubernetes.Clientset, error) {
	restCfg := &rest.Config{
		Host:        c.data.Server,
		BearerToken: c.data.Token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	return kubernetes.NewForConfig(restCfg)
}

func (c *K8SConfig) Term(w *Window) {
	cli, err := c.clientset()
	if err != nil {
		w.showError(err)
		return
	}
	st := &k8sDialog{win: w, cfg: c, cli: cli}
	w.k8s = st
	go st.loadNamespaces()
}

// k8sDialog is the dialog that picks a pod's container to connect to.
type k8sDialog struct {
	win  *Window
	open bool
	cfg  *K8SConfig
	cli  *kubernetes.Clientset

	namespaces []string
	pods       []string
	containers []string

	ns, pod, ct string
	err         error
	loadingPods bool
	loadingCTs  bool
}

func (st *k8sDialog) loadNamespaces() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	list, err := st.cli.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	st.win.onMain(func() {
		st.err = err
		if err == nil {
			s, _ := stream.New(list.Items)
			s.Map(func(n corev1.Namespace) string {
				return n.ObjectMeta.Name
			}).ToSlice(&st.namespaces)
		}
	})
}

func (st *k8sDialog) loadPods() {
	st.win.onMain(func() { st.loadingPods = true })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	list, err := st.cli.CoreV1().Pods(st.ns).List(ctx, metav1.ListOptions{})
	st.win.onMain(func() {
		st.err, st.loadingPods = err, false
		if err == nil {
			s, _ := stream.New(list.Items)
			s.Map(func(n corev1.Pod) string {
				return n.ObjectMeta.Name
			}).Limit(20).ToSlice(&st.pods)
		}
	})
}

func (st *k8sDialog) loadContainers() {
	st.win.onMain(func() { st.loadingCTs = true })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pod, err := st.cli.CoreV1().Pods(st.ns).Get(ctx, st.pod, metav1.GetOptions{})
	st.win.onMain(func() {
		st.err, st.loadingCTs = err, false
		if err == nil {
			s, _ := stream.New(pod.Spec.Containers)
			s.Map(func(n corev1.Container) string {
				return n.Name
			}).Limit(20).ToSlice(&st.containers)
		}
	})
}

func (st *k8sDialog) dialog(c *ui.Context) {
	ui.Modal(c, &st.open, func() {
		ui.Column(c).Width(460).Padding(16).Gap(12).Children(func() {
			ui.Text(c, locales.DockerTitle())
			if st.err != nil {
				ui.Text(c, st.err.Error()).TextColor(c.Theme().Danger)
			}
			ui.Form(c, func() {
				ui.Field(c, locales.FieldNamespace(), func() {
					if ui.Select(c, &st.ns, st.namespaces).Changed() {
						st.pods, st.pod, st.containers, st.ct = nil, "", nil, ""
						go st.loadPods()
					}
				})
				ui.Field(c, locales.FieldPod(), func() {
					if ui.Select(c, &st.pod, st.pods).Changed() {
						st.containers, st.ct = nil, ""
						go st.loadContainers()
					}
				})
				ui.Field(c, locales.FieldContainer(), func() {
					ui.Select(c, &st.ct, st.containers)
				})
			})
			switch {
			case st.loadingPods || st.loadingCTs:
				ui.Row(c).Gap(8).Children(func() {
					ui.Spinner(c)
					ui.Text(c, locales.K8sLoading())
				})
			case st.ns == "" || st.pod == "" || st.ct == "":
				ui.Text(c, locales.K8sHint()).TextColor(c.Theme().TextMuted)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.BtnCancel()).Clicked() {
					st.open = false
				}
				ok := ui.PrimaryButton(c, locales.BtnConnect())
				if st.ns == "" || st.pod == "" || st.ct == "" {
					ok.Disabled(true)
				}
				if ok.Clicked() {
					st.connect()
				}
			})
		})
	})
}

func (st *k8sDialog) connect() {
	w := st.win
	if st.ns == "" || st.pod == "" || st.ct == "" {
		return
	}
	restCfg := &rest.Config{
		Host:        st.cfg.data.Server,
		BearerToken: st.cfg.data.Token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	req := st.cli.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(st.pod).
		Namespace(st.ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: st.ct,
			Command:   []string{"bash"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restCfg, "POST", req.URL())
	if err != nil {
		w.showError(err)
		return
	}

	q := &sizeQueue{ch: make(chan *remotecommand.TerminalSize, 1)}
	conn := newK8SConn(q)

	tab, _, err := w.newTab(st.pod, st.cfg, func() { conn.Close() }, terminal.Options{Conn: conn})
	if err != nil {
		w.showError(err)
		conn.Close()
		return
	}

	go func() {
		err := executor.Stream(remotecommand.StreamOptions{
			Stdin:             conn.stdin,
			Stdout:            conn.stdout,
			Stderr:            conn.stdout,
			Tty:               true,
			TerminalSizeQueue: q,
		})
		if err != nil {
			w.showError(err)
		}
	}()

	w.onMain(func() {
		st.open = false
		w.addTermTab(tab)
	})
	w.showToast(locales.ToastConnected(st.pod), "success")
}

// sizeQueue tells the executor how big the terminal is; the terminal calls
// the queue's Resize through k8sConn as its view resizes it.
type sizeQueue struct {
	ch chan *remotecommand.TerminalSize
}

func (q *sizeQueue) Resize(cols, rows int) error {
	select {
	case <-q.ch: // drop a size nobody read
	default:
	}
	q.ch <- &remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}
	return nil
}

func (q *sizeQueue) Next() *remotecommand.TerminalSize {
	return <-q.ch
}

// k8sConn connects a terminal to a K8S exec stream: what the terminal
// writes the executor reads, what it writes the terminal shows, and a
// resize of the view becomes a resize of the remote terminal.
type k8sConn struct {
	in  *io.PipeWriter // what the terminal writes, for the executor to read
	out *io.PipeReader // what the executor wrote, for the terminal to show

	stdin  io.ReadCloser // the read end of in, which the executor reads
	stdout io.WriteCloser

	q *sizeQueue

	closeOnce sync.Once
}

func newK8SConn(q *sizeQueue) *k8sConn {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	return &k8sConn{
		in:     inWriter,
		out:    outReader,
		stdin:  inReader,
		stdout: outWriter,
		q:      q,
	}
}

func (c *k8sConn) Read(p []byte) (int, error) {
	return c.out.Read(p)
}

func (c *k8sConn) Write(p []byte) (int, error) {
	return c.in.Write(p)
}

func (c *k8sConn) Resize(cols, rows int) error {
	return c.q.Resize(cols, rows)
}

func (c *k8sConn) Close() error {
	c.closeOnce.Do(func() {
		c.in.Close()
		c.out.Close()
		c.stdin.Close()
		c.stdout.Close()
	})
	return nil
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/tk103331/stream"
	"goshell/locales"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type K8SConfigData struct {
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Server      string `json:"server,omitempty"`
	Token       string `json:"token,omitempty"`       // 加密存储
	InsecureTLS bool   `json:"insecureTLS,omitempty"` // 是否跳过TLS验证（默认false）
	Source      string `json:"source,omitempty"`      // "kubeconfig" or "token"
	Kubeconfig  string `json:"kubeconfig,omitempty"`
	Context     string `json:"context,omitempty"`
}

// getToken 返回解密后的 token
func (d *K8SConfigData) getToken() (string, error) {
	return decryptString(d.Token)
}

// setToken 加密并设置 token
func (d *K8SConfigData) setToken(token string) (string, error) {
	encrypted, err := encryptString(token)
	if err != nil {
		return "", err
	}
	d.Token = encrypted
	return encrypted, nil
}

type K8SConfig struct {
	data *K8SConfigData

	tokenPlain string // the token as the form edits it, encrypted on save
	kube       bool   // the form's connection kind
}

func (c *K8SConfig) Name() string      { return c.data.Name }
func (c *K8SConfig) Type() string      { return "k8s" }
func (c *K8SConfig) Data() interface{} { return c.data }

func (c *K8SConfig) Apply() {
	c.data.Type = c.Type()
	if c.kube {
		c.data.Source = "kubeconfig"
	} else {
		c.data.Source = "token"
	}
	// a token typed in this session is encrypted; a blank one leaves the
	// stored token alone, as when editing without retyping it
	if c.tokenPlain != "" || c.data.Token == "" {
		if _, err := c.data.setToken(c.tokenPlain); err != nil {
			log.Printf("Failed to encrypt token: %v", err)
			c.data.Token = c.tokenPlain
		}
	}
}

func (c *K8SConfig) Prefill() {
	// tokens stay hidden when editing: retype to change one
	c.tokenPlain = ""
	c.kube = c.data.Source == "kubeconfig" ||
		(c.data.Source == "" && c.data.Kubeconfig != "")
}

func (c *K8SConfig) Validate() string {
	if c.data.Name == "" {
		return locales.ValNameRequired()
	}
	if c.kube {
		if strings.TrimSpace(c.data.Kubeconfig) == "" {
			return locales.ValKubeconfigRequired()
		}
		if _, err := os.Stat(expandKubeconfigPath(c.data.Kubeconfig)); err != nil {
			return locales.ValKubeconfigNotFound(c.data.Kubeconfig)
		}
		return ""
	}
	if c.data.Server == "" {
		return locales.ValServerRequired()
	}
	if c.data.Token == "" && c.tokenPlain == "" {
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
	ui.Field(ctx, locales.FieldK8sSource(), func() {
		ui.RadioGroup(ctx, func() {
			ui.Radio(ctx, &c.kube, true, locales.K8sSourceKubeconfig())
			ui.Radio(ctx, &c.kube, false, locales.K8sSourceToken())
		})
	})
	if c.kube {
		ui.Field(ctx, locales.FieldKubeconfig(), func() {
			ui.Row(ctx).FillWidth().Gap(6).Children(func() {
				ui.TextInput(ctx, &c.data.Kubeconfig).Grow(1).Placeholder("~/.kube/config")
				if ui.Button(ctx, locales.BtnChooseKubeconfig()).Clicked() {
					go func() {
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Title:       locales.ChooseKubeconfigTitle(),
							ButtonLabel: locales.BtnUseKubeconfig(),
						})
						if err != nil || len(paths) == 0 {
							return
						}
						w.onMain(func() { c.data.Kubeconfig = paths[0] })
					}()
				}
			})
		})
		ui.Field(ctx, locales.FieldKubeContext(), func() {
			ui.TextInput(ctx, &c.data.Context).Placeholder(locales.KubeContextPlaceholder())
		})
		return
	}
	ui.Field(ctx, locales.FieldServer(), func() { ui.TextInput(ctx, &c.data.Server) })
	ui.Field(ctx, locales.FieldToken(), func() {
		e := ui.TextArea(ctx, &c.tokenPlain).Lines(3, 6)
		if editing {
			e.Placeholder("••••••••")
		}
	})
	ui.Field(ctx, locales.FieldSecurity(), func() {
		ui.Checkbox(ctx, &c.data.InsecureTLS, locales.InsecureTLS())
	})
}

// useKubeconfig says whether the saved config reads a kubeconfig, which
// survives the form's own state.
func (c *K8SConfig) useKubeconfig() bool {
	return c.data.Source == "kubeconfig" ||
		(c.data.Source == "" && c.data.Kubeconfig != "")
}

// restConfig builds the client configuration from a kubeconfig or the saved
// server and token.
func (c *K8SConfig) restConfig() (*rest.Config, error) {
	if c.useKubeconfig() {
		path := expandKubeconfigPath(c.data.Kubeconfig)
		if path == "" {
			return nil, errors.New("k8s: kubeconfig is required")
		}
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
		overrides := &clientcmd.ConfigOverrides{}
		if c.data.Context != "" {
			overrides.CurrentContext = c.data.Context
		}
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	}
	token, err := secretOf(c.data.Token)
	if err != nil {
		return nil, err
	}
	return &rest.Config{
		Host:        c.data.Server,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: c.data.InsecureTLS,
		},
	}, nil
}

func (c *K8SConfig) clientset() (*kubernetes.Clientset, *rest.Config, error) {
	restCfg, err := c.restConfig()
	if err != nil {
		return nil, nil, err
	}
	cli, err := kubernetes.NewForConfig(restCfg)
	return cli, restCfg, err
}

// expandKubeconfigPath expands ~ at the start of a path, as the file picker
// and a typed path may hold it.
func expandKubeconfigPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

// defaultKubeconfig is the user's kubeconfig, when one exists.
func defaultKubeconfig() string {
	path := os.Getenv("KUBECONFIG")
	if path != "" {
		path, _, _ = strings.Cut(path, string(os.PathListSeparator))
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, ".kube", "config")
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func (c *K8SConfig) Term(w *Window) {
	cli, restCfg, err := c.clientset()
	if err != nil {
		w.showError(err)
		return
	}
	st := &k8sDialog{win: w, cfg: c, cli: cli, rest: restCfg, open: true}
	w.k8s = st
	go st.loadNamespaces()
}

// k8sDialog is the dialog that picks a pod's container to connect to.
type k8sDialog struct {
	win  *Window
	open bool
	cfg  *K8SConfig
	cli  *kubernetes.Clientset
	rest *rest.Config

	namespaces []string
	pods       []string
	containers []string

	ns, pod, ct       string
	err               error
	loadingNamespaces bool
	loadingPods       bool
	loadingCTs        bool
}

func (st *k8sDialog) loadNamespaces() {
	st.win.onMain(func() { st.loadingNamespaces, st.err = true, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	list, err := st.cli.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	st.win.onMain(func() {
		st.loadingNamespaces, st.err = false, err
		if err != nil {
			return
		}
		s, _ := stream.New(list.Items)
		s.Map(func(n corev1.Namespace) string {
			return n.ObjectMeta.Name
		}).ToSlice(&st.namespaces)
		sort.Strings(st.namespaces)
		if st.ns == "" {
			for _, ns := range st.namespaces {
				if ns == "default" {
					st.ns = ns
					break
				}
			}
			if st.ns == "" && len(st.namespaces) > 0 {
				st.ns = st.namespaces[0]
			}
		}
		if st.ns != "" {
			go st.loadPods()
		}
	})
}

func (st *k8sDialog) loadPods() {
	st.win.onMain(func() {
		st.loadingPods, st.err = true, nil
		st.pods, st.pod, st.containers, st.ct = nil, "", nil, ""
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	list, err := st.cli.CoreV1().Pods(st.ns).List(ctx, metav1.ListOptions{})
	st.win.onMain(func() {
		st.loadingPods, st.err = false, err
		if err != nil {
			return
		}
		s, _ := stream.New(list.Items)
		s.Map(func(n corev1.Pod) string {
			return n.ObjectMeta.Name
		}).ToSlice(&st.pods)
		sort.Strings(st.pods)
		if len(st.pods) > 0 {
			st.pod = st.pods[0]
			go st.loadContainers()
		}
	})
}

func (st *k8sDialog) loadContainers() {
	st.win.onMain(func() {
		st.loadingCTs, st.err = true, nil
		st.containers, st.ct = nil, ""
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pod, err := st.cli.CoreV1().Pods(st.ns).Get(ctx, st.pod, metav1.GetOptions{})
	st.win.onMain(func() {
		st.loadingCTs, st.err = false, err
		if err != nil {
			return
		}
		s, _ := stream.New(pod.Spec.Containers)
		s.Map(func(n corev1.Container) string {
			return n.Name
		}).ToSlice(&st.containers)
		sort.Strings(st.containers)
		if len(st.containers) > 0 {
			st.ct = st.containers[0]
		}
	})
}

func (st *k8sDialog) refresh() {
	go st.loadNamespaces()
}
func (st *k8sDialog) dialog(c *ui.Context) {
	lk := lookFor(c)
	ui.Modal(c, &st.open, func() {
		ui.Column(c).Width(520).Padding(20).Gap(12).Children(func() {
			dialogHeading(c, locales.K8sTitle())
			detail := st.cfg.data.Server
			if st.cfg.useKubeconfig() {
				detail = st.cfg.data.Kubeconfig
				if st.cfg.data.Context != "" {
					detail += " · " + st.cfg.data.Context
				}
			}
			if detail != "" {
				ui.Text(c, detail).FontSize(11.5).TextColor(lk.textFaint).SingleLine().Ellipsis("…")
			}

			ui.Form(c, func() {
				ui.Field(c, locales.FieldNamespace(), func() {
					if ui.Combobox(c, &st.ns, st.namespaces).Disabled(st.loadingNamespaces).Changed() {
						if st.ns != "" {
							go st.loadPods()
						}
					}
				})
				ui.Field(c, locales.FieldPod(), func() {
					if ui.Combobox(c, &st.pod, st.pods).Disabled(st.ns == "" || st.loadingPods).Changed() {
						if st.pod != "" {
							go st.loadContainers()
						}
					}
				})
				ui.Field(c, locales.FieldContainer(), func() {
					ui.Combobox(c, &st.ct, st.containers).Disabled(st.pod == "" || st.loadingCTs)
				})
			})

			if st.err != nil {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, st.err.Error()).FontSize(12).TextColor(lk.danger).Grow(1).LineHeight(1.35)
					if ui.Button(c, locales.BtnRetry()).Clicked() {
						st.refresh()
					}
				})
			} else if st.loadingNamespaces || st.loadingPods || st.loadingCTs {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(13, 13)
					ui.Text(c, locales.K8sLoading()).FontSize(12).TextColor(lk.textMuted)
				})
			} else if st.ns == "" || st.pod == "" || st.ct == "" {
				ui.Text(c, locales.K8sHint()).FontSize(12.5).TextColor(lk.textMuted)
			}

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, locales.BtnRefresh()).Clicked() {
					st.refresh()
				}
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
	restCfg := st.rest
	if restCfg == nil {
		w.showError(errors.New("k8s: no REST configuration"))
		return
	}
	// the marker lets the file sidebar find this shell's /proc entry
	token := fmt.Sprintf("goshell-%d", time.Now().UnixNano())
	command := "export GOSHELL_SESSION=" + token + "; exec /bin/sh"
	req := st.cli.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(st.pod).
		Namespace(st.ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: st.ct,
			Command:   []string{"/bin/sh", "-c", command},
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
	tab.useFiles(&k8sFiles{cli: st.cli, cfg: restCfg, ns: st.ns, pod: st.pod, ct: st.ct, marker: token})

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

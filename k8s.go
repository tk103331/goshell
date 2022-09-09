package main

import (
	"context"
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/fyne-io/terminal"
	"github.com/tk103331/stream"
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
	onOk func()
}

func (c *K8SConfig) Name() string {
	return c.data.Name
}

func (c *K8SConfig) Type() string {
	return "k8s"
}
func (c *K8SConfig) Load(s string) error {
	data := &K8SConfigData{}

	err := json.Unmarshal([]byte(s), data)
	if err != nil {
		return err
	}
	c.data = data
	return nil
}
func (c *K8SConfig) Data() interface{} {
	return c.data
}

func (c *K8SConfig) Form() *widget.Form {
	nameEntry := widget.NewEntry()
	serverEntry := widget.NewEntry()
	tokenEntry := widget.NewEntry()
	tokenEntry.MultiLine = true
	tokenEntry.Wrapping = fyne.TextWrapBreak
	data := c.data
	if data != nil {
		nameEntry.Text = data.Name
		nameEntry.Disable()
		serverEntry.Text = data.Server
		tokenEntry.Text = data.Token
	}
	c.onOk = func() {
		if c.data == nil {
			c.data = &K8SConfigData{Type: c.Type()}
		}
		c.data.Name = nameEntry.Text
		c.data.Server = serverEntry.Text
		c.data.Token = tokenEntry.Text
	}
	return widget.NewForm([]*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Server", serverEntry),
		widget.NewFormItem("Token", tokenEntry),
	}...)
}

func (c *K8SConfig) OnOk() {
	c.onOk()
}

type ExecOpt struct {
	Namespace string
	PodName   string
	Container string
}

func (c *K8SConfig) Term(win *Window) {
	cfg := c.data
	restCfg := &rest.Config{
		Host:            cfg.Server,
		BearerToken:     cfg.Token,
		BearerTokenFile: "",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		win.showError(err)
		return
	}
	namespaceList, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		win.showError(err)
		return
	}
	s, _ := stream.New(namespaceList.Items)
	namespaces := make([]string, 0)
	s.Map(func(n corev1.Namespace) string {
		return n.ObjectMeta.Name
	}).ToSlice(&namespaces)

	execOpt := &ExecOpt{}

	containerSelect := widget.NewSelect(namespaces, func(container string) {
		execOpt.Container = container
	})

	podSelect := widget.NewSelect(namespaces, func(pod string) {
		execOpt.PodName = pod
		containerSelect.ClearSelected()
		containerSelect.Options = []string{}

		go func() {
			pod, err2 := clientset.CoreV1().Pods(execOpt.Namespace).Get(context.Background(), pod, metav1.GetOptions{})
			if err2 != nil {
				win.showError(err2)
				return
			}
			s, _ := stream.New(pod.Spec.Containers)
			containers := make([]string, 0)
			s.Map(func(n corev1.Container) string {
				return n.Name
			}).Limit(20).ToSlice(&containers)
			containerSelect.Options = containers
		}()

	})

	namespaceSelect := widget.NewSelect(namespaces, func(namespace string) {
		execOpt.Namespace = namespace
		podSelect.ClearSelected()
		podSelect.Options = []string{}

		go func() {
			podList, err2 := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
			if err2 != nil {
				win.showError(err2)
				return
			}
			s, _ := stream.New(podList.Items)
			pods := make([]string, 0)
			s.Map(func(n corev1.Pod) string {
				return n.ObjectMeta.Name
			}).Limit(20).ToSlice(&pods)
			podSelect.Options = pods
		}()
	})
	namespaceSelect.Resize(fyne.Size{Height: 200})

	form := widget.NewForm(widget.NewFormItem("Namespace", namespaceSelect), widget.NewFormItem("Pod", podSelect), widget.NewFormItem("Container", containerSelect))
	dlg := dialog.NewCustomConfirm("Select a container", "Connect", "Cancel", form, func(b bool) {
		if b {
			req := clientset.CoreV1().RESTClient().Post().
				Resource("pods").
				Name(execOpt.PodName).
				Namespace(execOpt.Namespace).
				SubResource("exec").
				VersionedParams(&corev1.PodExecOptions{
					Container: execOpt.Container,
					Command:   []string{"bash"},
					Stdin:     true,
					Stdout:    true,
					Stderr:    true,
					TTY:       true,
				}, scheme.ParameterCodec)

			executor, err := remotecommand.NewSPDYExecutor(restCfg, "POST", req.URL())
			if err != nil {
				win.showError(err)
				return
			}

			termCfgChan := make(chan terminal.Config)

			term := NewTerm(execOpt.PodName, c)

			writer, reader := term.StartWithPipe(func(err error) {
				if err != nil {
					fmt.Println(err.Error())
					win.showError(err)
				}
			})

			go func() {

				err = executor.Stream(remotecommand.StreamOptions{
					Stdin:             reader,
					Stdout:            writer,
					Stderr:            writer,
					Tty:               true,
					TerminalSizeQueue: TermConfigSizeQueue(termCfgChan),
				})
				if err != nil {
					win.showError(err)
					return
				}
			}()

			term.AddConfigListener(func(config *terminal.Config) {
				if config != nil {
					go func() {
						termCfgChan <- *config
					}()
				}
			})
			win.AddTermTab(term)
		}
	}, win.win)
	dlg.Resize(fyne.Size{Width: 400})
	dlg.Show()
}

type TermConfigSizeQueue chan terminal.Config

func (t TermConfigSizeQueue) Next() *remotecommand.TerminalSize {
	cfg := <-t
	return &remotecommand.TerminalSize{Width: uint16(cfg.Columns), Height: uint16(cfg.Rows)}
}

func (t TermConfigSizeQueue) Send(cfg terminal.Config) {
	t <- cfg
}

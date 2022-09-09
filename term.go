package main

import (
	"github.com/fyne-io/terminal"
	"io"
	"log"
	"net"
)

type Term struct {
	name            string
	term            *terminal.Terminal
	stat            string
	local           bool
	sessionConfig   Config
	termConfig      *terminal.Config
	configListeners []func(*terminal.Config)
	closeListeners  []func()
}

func NewTerm(name string, cfg Config) *Term {
	term := terminal.New()
	tab := &Term{name: name, term: term, sessionConfig: cfg}
	tab.watchConfig()
	return tab
}

func (t *Term) Name() string {
	return t.name
}

func (t *Term) TermConfig() *terminal.Config {
	return t.termConfig
}

func (t *Term) SessionConfig() Config {
	return t.sessionConfig
}

func (t *Term) StartWithPipe(callback func(err error)) (io.WriteCloser, io.Reader) {

	pipe1Reader, pipe1Writer := io.Pipe()
	pipe2Reader, pipe2Writer := io.Pipe()

	go func() {
		err := t.term.RunWithConnection(pipe1Writer, pipe2Reader)
		if err != nil {
			callback(err)
		}
	}()
	return pipe2Writer, pipe1Reader
}

func (t *Term) RunWithConnection(conn net.Conn) error {
	return t.term.RunWithConnection(conn, conn)
}

func (t *Term) RunWithReaderAndWriter(in io.WriteCloser, out io.Reader) error {
	return t.term.RunWithConnection(in, out)
}

func (t *Term) RunWithReadWriteCloser(wr io.ReadWriteCloser) error {
	return t.term.RunWithConnection(wr, wr)
}

func (t *Term) Exit() {
	t.term.Exit()
	for _, listener := range t.closeListeners {
		listener()
	}
}

func (t *Term) Send(txt string) {
	t.term.Write([]byte(txt))
}

func (t *Term) FocusGained() {
	if t.term != nil {
		t.term.FocusGained()
	}
}

func (t *Term) AddConfigListener(fn func(config *terminal.Config)) {
	if fn != nil {
		t.configListeners = append(t.configListeners, fn)
	}
}

func (t *Term) AddCloseListener(fn func()) {
	if fn != nil {
		t.closeListeners = append(t.closeListeners, fn)
	}
}

func (t *Term) watchConfig() {
	cfgChan := make(chan terminal.Config)
	t.term.AddListener(cfgChan)
	go func() {
		for {
			select {
			case cfg := <-cfgChan:
				t.termConfig = &cfg
				for _, listener := range t.configListeners {
					listener(t.termConfig)
				}
			}
		}
	}()
}

func NewLocalTerm() *Term {
	term := terminal.New()
	go func() {
		err := term.RunLocalShell()
		if err != nil {
			log.Println(err)
			return
		}
	}()

	t := &Term{name: "local", term: term}
	t.watchConfig()
	return t
}

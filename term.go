package goshell

import (
	"fyne.io/fyne/v2/container"
	"github.com/fyne-io/terminal"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"strconv"
)

type Term struct {
	name string
	term *terminal.Terminal
	tab container.TabItem
	stat string
	session *ssh.Session
	local bool
}

func (t *Term) send(txt string) {
	t.term.Write([]byte(txt))
}

func (t *Term) FocusGained() {
	if t.term != nil {
		t.term.FocusGained()
	}
}

func newLocalTerm() (*Term,error) {
	term := terminal.New()
	go func() {
		err := term.RunLocalShell()
		if err != nil {
			log.Println(err)
			return
		}
	}()
	return &Term{name: "local", term: term, local: true},nil
}

func newSSHTerm(conf *Config) (*Term,error) {


	c := ssh.ClientConfig{User: conf.User, Auth: []ssh.AuthMethod{
		ssh.Password(conf.Pswd),
	}, HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}}

	addr := conf.Host + ":" + strconv.Itoa(conf.Port)
	conn, err := ssh.Dial("tcp", addr, &c)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	session, err := conn.NewSession()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	err = session.RequestPty("xterm-color", 24, 80, modes)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	in, err := session.StdinPipe()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	out, err := session.StdoutPipe()
	if err != nil {
		log.Println(err)
		return nil, err
	}

	term := terminal.New()
	go func() {
		err = term.RunWithConnection(in, out)
		if err != nil {
			log.Println(err)
		}
		session.Close()
	}()

	go func() {
		err := session.Shell()
		if err != nil {
			log.Println(err)
		}
	}()
	return &Term{name: conf.Name, term: term, session: session, local: false},nil
}

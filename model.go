package fyneshell

type Config struct {
	Name string
	Host string
	Port int
	User string
	Pswd string
}

func newConfig(name string) *Config {
	return &Config{Name: name}
}

type Cmd struct {
	Name string
	Text string
	Icon string
}

func newCmd(name, text string) *Cmd {
	return &Cmd{Name: name, Text: text}
}
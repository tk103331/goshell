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
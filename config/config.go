package config

import (
	"github.com/namsral/flag"
)

type Config struct {
	DataPath string
	LogLevel string
}

// Load loads the configs from the given arguments
func (c *Config) Load(args []string) error {
	fs := flag.NewFlagSet("wdb-server", flag.ContinueOnError)
	fs.StringVar(&c.DataPath, "wdb-data-path", "", "data path")
	fs.StringVar(&c.LogLevel, "log-level", "debug", "log level")
	err := fs.Parse(args)
	return err
}

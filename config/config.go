package config

import (
	"github.com/namsral/flag"
)

type Config struct {
	DataPath         string
	LogLevel         string
	DBMigrationsPath string
	DBConnUri        string
	SecretKey        string
}

// Load loads the configs from the given arguments
func (c *Config) Load(args []string) error {
	fs := flag.NewFlagSet("wdb-server", flag.ContinueOnError)
	fs.StringVar(&c.DataPath, "wdb-data-path", "", "data path")
	fs.StringVar(&c.LogLevel, "log-level", "info", "log level")
	fs.StringVar(&c.DBMigrationsPath, "db-migrations-path", "", "the path where migrations are stored")
	fs.StringVar(&c.DBConnUri, "db-conn-uri", "", "the db connection URI")
	fs.StringVar(&c.SecretKey, "secret-key", "", "the secret key for JWT signing")
	err := fs.Parse(args)
	return err
}

package contracts

import (
	contractsconfig "github.com/gradii/framework/contracts/config"
)

type ConfigBuilder interface {
	Config() contractsconfig.Config
	Connection() string
	Readers() []FullConfig
	Writers() []FullConfig
}

// Config Used in config/database.go
type Config struct {
	Dsn      string
	Host     string
	Port     int
	Database string
	Username string
	Password string
}

// FullConfig Fill the default value for Config
type FullConfig struct {
	Charset  string
	Timezone string
	Config
	Connection string
	Driver     string
	Prefix     string
}

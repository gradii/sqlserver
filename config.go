package sqlserver

import (
	"fmt"

	"github.com/gradii/framework/contracts/config"

	"github.com/gradii/sqlserver/contracts"
)

type Config struct {
	config     config.Config
	connection string
}

func NewConfig(config config.Config, connection string) *Config {
	return &Config{
		config:     config,
		connection: connection,
	}
}

func (r *Config) Config() config.Config {
	return r.config
}

func (r *Config) Connection() string {
	return r.connection
}

func (r *Config) Readers() []contracts.FullConfig {
	configs := r.config.Get(fmt.Sprintf("database.connections.%s.read", r.connection))
	if readConfigs, ok := configs.([]contracts.Config); ok {
		return r.fillDefault(readConfigs)
	}

	return nil
}

func (r *Config) Writers() []contracts.FullConfig {
	configs := r.config.Get(fmt.Sprintf("database.connections.%s.write", r.connection))
	if writeConfigs, ok := configs.([]contracts.Config); ok {
		return r.fillDefault(writeConfigs)
	}

	// Use default db configuration when write is empty
	return r.fillDefault([]contracts.Config{{}})
}

func (r *Config) fillDefault(configs []contracts.Config) []contracts.FullConfig {
	if len(configs) == 0 {
		return nil
	}

	var fullConfigs []contracts.FullConfig
	for _, config := range configs {
		fullConfig := contracts.FullConfig{
			Config:     config,
			Connection: r.connection,
			Driver:     Name,
			Prefix:     r.config.GetString(fmt.Sprintf("database.connections.%s.prefix", r.connection)),
		}

		// If read or write is empty, use the default config
		if fullConfig.Dsn == "" {
			fullConfig.Dsn = r.config.GetString(fmt.Sprintf("database.connections.%s.dsn", r.connection))
		}
		if fullConfig.Host == "" {
			fullConfig.Host = r.config.GetString(fmt.Sprintf("database.connections.%s.host", r.connection))
		}
		if fullConfig.Port == 0 {
			fullConfig.Port = r.config.GetInt(fmt.Sprintf("database.connections.%s.port", r.connection))
		}
		if fullConfig.Username == "" {
			fullConfig.Username = r.config.GetString(fmt.Sprintf("database.connections.%s.username", r.connection))
		}
		if fullConfig.Password == "" {
			fullConfig.Password = r.config.GetString(fmt.Sprintf("database.connections.%s.password", r.connection))
		}
		if fullConfig.Database == "" {
			fullConfig.Database = r.config.GetString(fmt.Sprintf("database.connections.%s.database", r.connection))
		}
		if fullConfig.Charset == "" {
			fullConfig.Charset = r.config.GetString(fmt.Sprintf("database.connections.%s.charset", r.connection))
		}
		if fullConfig.Timezone == "" {
			timezone := r.config.GetString(fmt.Sprintf("database.connections.%s.timezone", r.connection))
			if timezone == "" {
				timezone = r.config.GetString("app.timezone", "UTC")
			}
			fullConfig.Timezone = timezone
		}
		fullConfigs = append(fullConfigs, fullConfig)
	}

	return fullConfigs
}

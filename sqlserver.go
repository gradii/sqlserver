package sqlserver

import (
	"github.com/gradii/framework/contracts/config"
	"github.com/gradii/framework/contracts/database"
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/contracts/log"
	"github.com/gradii/framework/contracts/process"
	"github.com/gradii/framework/contracts/testing/docker"
	"github.com/gradii/sqlserver/contracts"
	drivererrors "github.com/gradii/sqlserver/errors"
	"sync"
)

var _ driver.Driver = (*Sqlserver)(nil)
var _ driver.DriverWithDialect = (*Sqlserver)(nil)

type Sqlserver struct {
	config  contracts.ConfigBuilder
	mu      sync.RWMutex
	version func() (string, error)
}

func NewSqlserver(configuration config.Config, _ log.Log, _ process.Process, connection string) *Sqlserver {
	return &Sqlserver{config: NewConfig(configuration, connection)}
}
func (r *Sqlserver) Docker() (docker.DatabaseDriver, error) {
	return nil, drivererrors.DockerNotSupported
}
func (r *Sqlserver) Dialect() driver.Dialect { return NewDialect() }
func (r *Sqlserver) SetServerVersion(resolve func() (string, error)) {
	r.mu.Lock()
	r.version = resolve
	r.mu.Unlock()
}
func (r *Sqlserver) Grammar() driver.Grammar {
	prefix := ""
	if writers := r.config.Writers(); len(writers) > 0 {
		prefix = writers[0].Prefix
	}
	grammar := NewGrammar(prefix)
	r.mu.RLock()
	resolver := r.version
	r.mu.RUnlock()
	grammar.SetVersionResolver(resolver)
	return grammar
}
func (r *Sqlserver) Pool() database.Pool {
	return database.Pool{Readers: r.fullConfigsToConfigs(r.config.Readers()), Writers: r.fullConfigsToConfigs(r.config.Writers())}
}
func (r *Sqlserver) Processor() driver.Processor { return NewProcessor() }
func (r *Sqlserver) fullConfigsToConfigs(values []contracts.FullConfig) []database.Config {
	configs := make([]database.Config, len(values))
	for i, c := range values {
		configs[i] = database.Config{Charset: c.Charset, Connection: c.Connection, Dsn: c.Dsn, Database: c.Database, Driver: Name, Host: c.Host, Password: c.Password, Port: c.Port, Prefix: c.Prefix, Username: c.Username, Timezone: c.Timezone}
	}
	return configs
}

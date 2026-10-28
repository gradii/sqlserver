package sqlserver

import (
	"database/sql/driver"
	"github.com/gradii/framework/contracts/database"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	contractsquery "github.com/gradii/framework/contracts/database/query"
)

type Dialect struct{}

func NewDialect() *Dialect      { return &Dialect{} }
func (d *Dialect) Name() string { return Name }
func (d *Dialect) NewConnector(config database.Config) (driver.Connector, error) {
	return NewConnector(config)
}
func (d *Dialect) NewQueryGrammar(options contractsdriver.GrammarOptions) contractsquery.Grammar {
	return NewQueryGrammar(options.Prefix)
}
func (d *Dialect) NewProcessor() contractsquery.Processor { return NewQueryProcessor() }
func (d *Dialect) ServerVersionSQL() string {
	return "select cast(serverproperty('ProductVersion') as varchar(128))"
}

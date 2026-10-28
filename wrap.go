package sqlserver

import (
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/support/collect"
)

type Wrap struct {
	*schema.Wrap
}

func NewWrap(prefix string) *Wrap {
	return &Wrap{
		Wrap: schema.NewWrap(prefix),
	}
}

func (r *Wrap) Quotes(value []string) []string {
	return collect.Map(value, func(v string, _ int) string {
		return "N" + r.Quote(v)
	})
}

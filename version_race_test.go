package sqlserver

import (
	"sync"
	"testing"

	frameworkconfig "github.com/gradii/framework/config"
)

func TestServerVersionResolverConcurrentAccess(t *testing.T) {
	config := frameworkconfig.NewApplication("")
	instance := NewSqlserver(config, nil, nil, "version_race")
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				instance.SetServerVersion(func() (string, error) { return "16.0.1", nil })
				instance.Grammar()
			}
		})
	}
	workers.Wait()
}

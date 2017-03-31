package database

import (
	"testing"
)

func TestListServicesInfo(t *testing.T) {
	if ormer == nil {
		t.Skip("orm:db is required")
	}

	services, err := ormer.ListServicesInfo()
	if err != nil {
		t.Error(err)
	}

	for i := range services {
		t.Logf("%d %+v\n", i, services[i].Service)
	}
}

package parser

import (
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type linkGenerator interface {
	generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error)
}

const (
	SM_UPP_UPSQLs = "SwitchManager_Upproxy_UpSQL"
	Proxy_Redis   = "Sentinel_Proxy_Redis"
)

func linkFactory(mode, nameOrID string, links []*structs.ServiceLink) (linkGenerator, error) {
	switch mode {

	case SM_UPP_UPSQLs:
		return newLinkUpSQL(nameOrID, links)

	case Proxy_Redis:
		return newLinkRedis(nameOrID, links)

	}

	return nil, errors.Errorf("Unsupported %s mode yet", mode)
}

func isDesignated(dest, unit string, spec *structs.ServiceSpec) bool {
	if dest == "" || dest == unit {
		return true
	}

	if spec == nil {
		return false
	}

	if spec.ID == dest || spec.Name == dest {
		return true
	}

	for i := range spec.Units {
		if (unit == "" || spec.Units[i].ID == unit ||
			spec.Units[i].Name == unit ||
			spec.Units[i].ContainerID == unit) &&

			(spec.Units[i].ID == dest ||
				spec.Units[i].Name == dest ||
				spec.Units[i].ContainerID == dest) {

			return true
		}
	}

	return false
}

package parser

import (
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"golang.org/x/net/context"
)

type linkGenerator interface {
	generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error)
}

const (
	SM_UPP_UPSQLs = "SwitchManager_Upproxy_UpSQL"
	Proxy_Redis   = "Sentinel_Proxy_Redis"
)

func linkFactory(mode string, links []*structs.ServiceLink) (linkGenerator, error) {
	switch mode {

	case SM_UPP_UPSQLs:
		return newLinkUpSQL(links)

	case Proxy_Redis:

		return newLinkRedis(links)

	default:

	}

	return nil, nil
}

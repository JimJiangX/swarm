package parser

import (
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type linkRedis struct {
	sentinel *structs.ServiceLink
	proxy    *structs.ServiceLink
	redis    []*structs.ServiceLink
}

func newLinkRedis(links []*structs.ServiceLink) (linkRedis, error) {
	obj := linkRedis{}

	if len(links) < 3 {
		return obj, errors.Errorf("invalid paramaters in %s mode", Proxy_Redis)
	}

	for i := range links {

		v, err := structs.ParseImage(links[i].Spec.Image)
		if err != nil {
			return obj, err
		}

		if links[i].Arch != (structs.Arch{}) {
			links[i].Spec.Arch = links[i].Arch
		}

		switch v.Name {
		case "upredis":
			if obj.redis == nil {
				obj.redis = make([]*structs.ServiceLink, 0, 2)
			}

			obj.redis = append(obj.redis, links[i])

		case "upredis-proxy":
			obj.proxy = links[i]

		case "sentinel":
			obj.sentinel = links[i]

		default:
			return obj, errors.Errorf("Unsupported image %s in link %s", v.Name, Proxy_Redis)
		}
	}

	return obj, nil
}

func (lr linkRedis) generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error) {
	resp := structs.ServiceLinkResponse{}

	return resp, nil
}

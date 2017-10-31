package parser

import (
	"fmt"
	"strings"

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

	if obj.proxy == nil || obj.redis == nil || obj.sentinel == nil {
		return obj, errors.Errorf("the condition is not satisfied mode %s", Proxy_Redis)
	}

	return obj, nil
}

func (lr linkRedis) generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error) {
	resp := structs.ServiceLinkResponse{}

	{
		cms, pr, err := getServiceConfigParser(ctx, client, lr.sentinel.Spec.ID, lr.sentinel.Spec.Image)
		if err != nil {
			return resp, err
		}

		addrs := make([]string, 0, len(lr.sentinel.Spec.Units))

		for _, u := range lr.sentinel.Spec.Units {

			cc := cms[u.ID]
			pr.clone(nil)

			err := pr.ParseData([]byte(cc.Content))
			if err != nil {
				return resp, err
			}

			port := pr.get("port")

			addrs = append(addrs, fmt.Sprintf("%s:%s", u.Networking[0].IP, port))
		}

		opts := make(map[string]map[string]interface{})
		opts[allUnitsEffect] = map[string]interface{}{
			"default::sentinels": strings.Join(addrs, stringAndString),
		}

		ulinks, err := generateServiceLink(ctx, client, *lr.proxy.Spec, opts)
		if err != nil {
			return resp, err
		}

		resp.Links = append(resp.Links, ulinks...)
	}

	return resp, nil
}

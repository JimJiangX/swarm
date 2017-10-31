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
	resp := structs.ServiceLinkResponse{
		Links: make([]structs.UnitLink, 0, 6),
	}
	// services addr
	sentinels := make([]string, 0, len(lr.sentinel.Spec.Units))
	proxys := make([]string, 0, len(lr.proxy.Spec.Units))
	redisClusters := make([]string, 0, len(lr.redis))

	{
		// proxy
		cms, pr, err := getServiceConfigParser(ctx, client, lr.proxy.Spec.ID, lr.proxy.Spec.Image)
		if err != nil {
			return resp, err
		}

		for _, u := range lr.proxy.Spec.Units {

			cc := cms[u.ID]
			pr.clone(nil)

			err := pr.ParseData([]byte(cc.Content))
			if err != nil {
				return resp, err
			}

			proxys = append(proxys, pr.get("listen"))
		}
	}

	{
		// sentinel
		cms, pr, err := getServiceConfigParser(ctx, client, lr.sentinel.Spec.ID, lr.sentinel.Spec.Image)
		if err != nil {
			return resp, err
		}

		for _, u := range lr.sentinel.Spec.Units {

			cc := cms[u.ID]
			pr.clone(nil)

			err := pr.ParseData([]byte(cc.Content))
			if err != nil {
				return resp, err
			}

			port := pr.get("port")

			sentinels = append(sentinels, fmt.Sprintf("%s:%s", u.Networking[0].IP, port))
		}
	}

	// redis
	for i := range lr.redis {
		cms, pr, err := getServiceConfigParser(ctx, client, lr.redis[i].Spec.ID, lr.redis[i].Spec.Image)
		if err != nil {
			return resp, err
		}

		redis := make([]string, 0, len(lr.redis[i].Spec.Units))

		for _, u := range lr.redis[i].Spec.Units {

			cc := cms[u.ID]
			pr.clone(nil)

			err := pr.ParseData([]byte(cc.Content))
			if err != nil {
				return resp, err
			}

			ip := pr.get("bind")
			port := pr.get("port")

			redis = append(redis, fmt.Sprintf("%s:%s", ip, port))
		}

		redisClusters = append(redisClusters, strings.Join(redis, ","))
	}

	// link commands,/root/link-init.sh -s ip:port,ip:port -p ip:port,ip:port -r ip:port,ip:port ip:port,ip:port
	linkCmd := make([]string, 6+len(redisClusters))
	linkCmd[0] = "/root/link-init.sh"
	linkCmd[1] = "-s"
	linkCmd[2] = strings.Join(sentinels, ",")
	linkCmd[3] = "-p"
	linkCmd[4] = strings.Join(proxys, ",")
	linkCmd[5] = "-r"
	copy(linkCmd[6:], redisClusters)

	{
		opts := make(map[string]map[string]interface{})
		opts[allUnitsEffect] = map[string]interface{}{
			"default::sentinels": strings.Join(sentinels, stringAndString),
		}

		ulinks, err := generateServiceLink(ctx, client, *lr.proxy.Spec, opts)
		if err != nil {
			return resp, err
		}

		for i := range ulinks {
			ulinks[i].Commands = linkCmd
			resp.Links = append(resp.Links, ulinks[i])
		}
	}
	{

		for _, u := range lr.sentinel.Spec.Units {
			ul := structs.UnitLink{
				NameOrID:  u.ID,
				ServiceID: u.ServiceID,
				Commands:  linkCmd,
			}

			resp.Links = append(resp.Links, ul)
		}
	}

	return resp, nil
}

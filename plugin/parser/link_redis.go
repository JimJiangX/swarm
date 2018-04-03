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
	nameOrID string
	sentinel *structs.ServiceLink
	proxy    *structs.ServiceLink
	redis    []*structs.ServiceLink
}

func newLinkRedis(nameOrID string, links []*structs.ServiceLink) (linkRedis, error) {
	obj := linkRedis{
		nameOrID: nameOrID,
	}

	if len(links) < 3 {
		return obj, errors.Errorf("invalid paramaters in %s mode", Proxy_Redis)
	}

	for i := range links {

		if links[i].Arch != (structs.Arch{}) {
			links[i].Spec.Arch = links[i].Arch
		}

		switch links[i].Spec.Image.Name {
		case "upredis":
			if obj.redis == nil {
				obj.redis = make([]*structs.ServiceLink, 0, 2)
			}

			obj.redis = append(obj.redis, links[i])

		case "urproxy":
			obj.proxy = links[i]

		case "sentinel":
			obj.sentinel = links[i]

		default:
			return obj, errors.Errorf("Unsupported image %s in link %s", links[i].Spec.Image.Image(), Proxy_Redis)
		}
	}

	if obj.proxy == nil || obj.redis == nil || obj.sentinel == nil {
		return obj, errors.Errorf("the condition is not satisfied mode %s", Proxy_Redis)
	}

	return obj, nil
}

func (lr linkRedis) generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error) {
	resp := structs.ServiceLinkResponse{
		Links:                make([]structs.UnitLink, 0, 6),
		ReloadServicesConfig: make([]string, 2+len(lr.redis)),
	}

	resp.ReloadServicesConfig[0] = lr.proxy.ID
	resp.ReloadServicesConfig[1] = lr.sentinel.ID
	for i := range lr.redis {
		resp.ReloadServicesConfig[2+i] = lr.redis[i].ID
	}

	// services addr
	sentinels := make([]string, 0, len(lr.sentinel.Spec.Units))
	proxys := make([]string, 0, len(lr.proxy.Spec.Units))
	redisClusters := make([]string, 0, len(lr.redis))

	var proxyCMS, sentinelCMS structs.ConfigsMap

	{
		// proxy
		cms, pr, err := getServiceConfigParser(ctx, client, lr.proxy.Spec.ID, lr.proxy.Spec.Image)
		if err != nil {
			return resp, err
		}

		proxyCMS = cms

		for _, u := range lr.proxy.Spec.Units {

			cc := cms[u.ID]
			pr = pr.clone(nil)

			err := pr.ParseData([]byte(cc.Content))
			if err != nil {
				return resp, err
			}

			val, _ := pr.get("listen")
			proxys = append(proxys, val)
		}
	}

	{
		// sentinel
		cms, pr, err := getServiceConfigParser(ctx, client, lr.sentinel.Spec.ID, lr.sentinel.Spec.Image)
		if err != nil {
			return resp, err
		}

		sentinelCMS = cms

		for _, u := range lr.sentinel.Spec.Units {

			cc := cms[u.ID]
			pr = pr.clone(nil)

			err := pr.ParseData([]byte(cc.Content))
			if err != nil {
				return resp, err
			}

			if len(u.Networking) == 0 {
				return resp, errors.Errorf("unit networking is required,unit=%s", u.Name)
			}

			port, _ := pr.get("port")
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
			pr = pr.clone(nil)

			err := pr.ParseData([]byte(cc.Content))
			if err != nil {
				return resp, err
			}

			ip, _ := pr.get("bind")
			port, _ := pr.get("port")
			redis = append(redis, fmt.Sprintf("%s:%s", ip, port))
		}

		redisClusters = append(redisClusters, strings.Join(redis, ","))
	}

	// link commands,/root/link-init.sh -s ip:port,ip:port -p ip:port,ip:port -r ip:port,ip:port#ip:port,ip:port
	linkCmd := []string{
		"/root/link-init.sh",
		"-s", strings.Join(sentinels, ","),
		"-p", strings.Join(proxys, ","),
		"-r", strings.Join(redisClusters, "#"),
	}

	if isDesignated(lr.nameOrID, "", lr.proxy.Spec) {

		opts := make(map[string]map[string]interface{})
		opts[allUnitsEffect] = map[string]interface{}{
			"default::sentinels": strings.Join(sentinels, stringAndString),
		}

		ulinks, err := generateServiceLink(ctx, client, *lr.proxy.Spec, opts)
		if err != nil {
			return resp, err
		}

		for i := range ulinks {
			if !isDesignated(lr.nameOrID, ulinks[i].NameOrID, lr.proxy.Spec) {
				continue
			}

			ulinks[i].Commands = linkCmd
			resp.Links = append(resp.Links, ulinks[i])
		}
	}

	if isDesignated(lr.nameOrID, "", lr.sentinel.Spec) {

		for _, u := range lr.sentinel.Spec.Units {
			if !isDesignated(lr.nameOrID, u.ID, lr.sentinel.Spec) {
				continue
			}

			ul := structs.UnitLink{
				NameOrID:  u.ID,
				ServiceID: u.ServiceID,
				Commands:  linkCmd,
			}

			resp.Links = append(resp.Links, ul)
		}
	}

	// start service

	if isDesignated(lr.nameOrID, "", lr.proxy.Spec) {

		for _, u := range lr.proxy.Spec.Units {
			if !isDesignated(lr.nameOrID, u.ID, lr.proxy.Spec) {
				continue
			}

			ul := structs.UnitLink{
				NameOrID:  u.ID,
				ServiceID: u.ServiceID,
				Commands:  proxyCMS[u.ID].GetCmd(structs.StartServiceCmd),
			}

			resp.Links = append(resp.Links, ul)
		}
	}

	if isDesignated(lr.nameOrID, "", lr.sentinel.Spec) {

		for _, u := range lr.sentinel.Spec.Units {
			if !isDesignated(lr.nameOrID, u.ID, lr.sentinel.Spec) {
				continue
			}

			ul := structs.UnitLink{
				NameOrID:  u.ID,
				ServiceID: u.ServiceID,
				Commands:  sentinelCMS[u.ID].GetCmd(structs.StartServiceCmd),
			}

			resp.Links = append(resp.Links, ul)
		}
	}

	return resp, nil
}

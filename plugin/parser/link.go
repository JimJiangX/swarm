package parser

import (
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

type linkGenerator interface {
	generateLinkConfig() (structs.ServiceLinkResponse, error)
}

const (
	SM_UPP_UPSQL = "SwitchManager_Upproxy_UpSQL"
	Proxy_Redis  = "proxy_redis"
)

func linkFactory(mode string, links []*structs.ServiceLink) (linkGenerator, error) {
	switch mode {

	case SM_UPP_UPSQL:
		return newLinkUpSQL(links)

	case Proxy_Redis:

	default:

	}

	return nil, nil
}

type linkUpSQL struct {
	swm   *structs.ServiceLink
	proxy *structs.ServiceLink
	sql   *structs.ServiceLink
}

func newLinkUpSQL(links []*structs.ServiceLink) (linkUpSQL, error) {
	obj := linkUpSQL{}

	if len(links) != 3 {
		return obj, errors.Errorf("invaild paramaters in %s mode", SM_UPP_UPSQL)
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
		case "upsql":

			obj.sql = links[i]

		case "upproxy":
			obj.proxy = links[i]

		case "switch_manager":
			obj.swm = links[i]

		default:
			return obj, errors.Errorf("Unsupported image %s in link %s", v.Name, SM_UPP_UPSQL)
		}
	}

	return obj, nil
}

func (sql linkUpSQL) generateLinkConfig() (structs.ServiceLinkResponse, error) {
	return structs.ServiceLinkResponse{}, nil
}

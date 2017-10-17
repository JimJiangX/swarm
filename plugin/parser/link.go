package parser

import (
	"net"

	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const allUnitsEffect = "ALL_UNITS"

type linkGenerator interface {
	generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error)
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
		return obj, errors.Errorf("invalid paramaters in %s mode", SM_UPP_UPSQL)
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

func (sql linkUpSQL) generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error) {
	resp := structs.ServiceLinkResponse{
		Links: make([]structs.UnitLink, 0, 5),
	}

	//	{
	//		opts := make(map[string]map[string]interface{})

	//		// set options

	//		ulinks, err := generateServiceLink(ctx, client, *sql.sql.Spec, opts)
	//		if err != nil {
	//			return resp, err
	//		}

	//		resp.Links = append(resp.Links, ulinks...)
	//	}

	{
		opts := make(map[string]map[string]interface{})
		// set options
		{
			ip := sql.swm.Spec.Units[0].Networking[0].IP

			swmc, err := getServiceUnitConfig(ctx, client, sql.swm.Spec.ID, sql.swm.Spec.Units[0].ID)
			if err != nil {
				return resp, err
			}

			pr, err := factory(sql.swm.Spec.Image)
			if err != nil {
				return resp, err
			}

			pr = pr.clone(nil)

			err = pr.ParseData([]byte(swmc.Content))
			if err != nil {
				return resp, err
			}

			port := pr.get("proxyport")

			opts[allUnitsEffect] = map[string]interface{}{"adm-cli::adm-svr-address": net.JoinHostPort(ip, port)}
		}

		ulinks, err := generateServiceLink(ctx, client, *sql.proxy.Spec, opts)
		if err != nil {
			return resp, err
		}

		resp.Links = append(resp.Links, ulinks...)
	}

	//	{
	//		opts := make(map[string]map[string]interface{})

	//		// set options

	//		ulinks, err := generateServiceLink(ctx, client, *sql.swm.Spec, opts)
	//		if err != nil {
	//			return resp, err
	//		}
	//		{
	//			// TODO:generate switch_manager init topoloy request

	//		}

	//		resp.Links = append(resp.Links, ulinks...)
	//	}

	//	resp.Compose = []string{sql.sql.ID, sql.proxy.ID, sql.swm.ID}

	return resp, nil
}

func generateServiceLink(ctx context.Context,
	client kvstore.Store,
	spec structs.ServiceSpec,
	opts map[string]map[string]interface{}) ([]structs.UnitLink, error) {

	cm, err := generateServiceConfigs(ctx, client, spec, "", opts)
	if err != nil {
		return nil, err
	}

	ulinks := make([]structs.UnitLink, 0, len(spec.Units))

	for _, cc := range cm {
		ulinks = append(ulinks, structs.UnitLink{
			NameOrID:      cc.ID,
			ServiceID:     spec.ID,
			ConfigFile:    cc.ConfigFile,
			ConfigContent: cc.Content,
			Commands:      cc.Cmds[structs.StartServiceCmd], // TODO:
		})
	}

	return ulinks, nil
}

package parser

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/vars"
	"github.com/pkg/errors"
	swm_structs "github.com/yiduoyunQ/sm/sm-svr/structs"
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

	{
		// sql
		if sql.sql != nil {
			for _, u := range sql.sql.Spec.Units {
				resp.Links = append(resp.Links, structs.UnitLink{
					NameOrID:  u.ID,
					ServiceID: sql.sql.Spec.ID,
					Commands:  []string{"/root/serv", "start"}, // TODO:
				})
			}
		}
	}

	swmCM, swmPr, err := getServiceConfigParser(ctx, client, sql.swm.Spec.ID, sql.swm.Spec.Image)
	if err != nil {
		return resp, err
	}

	swmAddr := ""
	swmc := swmCM[sql.swm.Spec.Units[0].ID]
	err = swmPr.ParseData([]byte(swmc.Content))
	if err != nil {
		return resp, err
	}

	{
		// proxy
		opts := make(map[string]map[string]interface{})
		// set options
		{
			port := swmPr.get("proxyport")
			ip := sql.swm.Spec.Units[0].Networking[0].IP
			swmAddr = net.JoinHostPort(ip, port)

			opts[allUnitsEffect] = map[string]interface{}{"adm-cli::adm-svr-address": swmAddr}
		}

		ulinks, err := generateServiceLink(ctx, client, *sql.proxy.Spec, opts)
		if err != nil {
			return resp, err
		}

		resp.Links = append(resp.Links, ulinks...)
	}

	{
		// swm
		if sql.swm != nil {

			body, err := swmInitTopology(ctx, client, sql)
			if err != nil {
				return resp, err
			}

			for _, u := range sql.swm.Spec.Units {
				resp.Links = append(resp.Links, structs.UnitLink{
					NameOrID:  u.ID,
					ServiceID: sql.swm.Spec.ID,
					Commands:  swmc.Cmds[structs.StartServiceCmd],
					Request: &structs.HTTPRequest{
						Method: http.MethodPost,
						URL:    "http://" + swmAddr + "/init",
						Body:   body,
						Header: map[string][]string{
							"Content-Type": {"application/json"},
						},
					},
				})
			}
		}
	}

	return resp, nil
}

func getServiceConfigParser(ctx context.Context, kvc kvstore.Store, service, image string) (structs.ConfigsMap, parser, error) {
	cm, err := getConfigMapFromStore(ctx, kvc, service)
	if err != nil {
		return nil, nil, err
	}

	pr, err := factory(image)
	if err != nil {
		return cm, nil, err
	}

	pr = pr.clone(nil)

	return cm, pr, err
}

func swmInitTopology(ctx context.Context, kvc kvstore.Store, sql linkUpSQL) ([]byte, error) {
	if len(sql.sql.Spec.Units) == 0 || len(sql.proxy.Spec.Units) == 0 || len(sql.swm.Spec.Units) == 0 {
		return nil, nil
	}

	proxyCM, proxyPr, err := getServiceConfigParser(ctx, kvc, sql.proxy.Spec.ID, sql.proxy.Spec.Image)
	if err != nil {
		return nil, err
	}

	proxyGroup := make(map[string]*swm_structs.ProxyInfo, len(sql.proxy.Spec.Units))
	for _, u := range sql.proxy.Spec.Units {
		cc := proxyCM[u.ID]
		proxyPr = proxyPr.clone(nil)
		proxyPr.ParseData([]byte(cc.Content))

		addr := proxyPr.get("upsql-proxy::proxy-address") // proxy-address = <proxy_ip_addr>:<proxy_data_port>
		parts := strings.SplitN(addr, ":", 2)

		proxyGroup[u.Name] = &swm_structs.ProxyInfo{
			Id:   strconv.Itoa(int(utils.IPToUint32(parts[0]))),
			Name: u.Name,
			Ip:   parts[0],
			Port: parts[1],
		}
	}

	sqlCM, sqlPr, err := getServiceConfigParser(ctx, kvc, sql.sql.Spec.ID, sql.sql.Spec.Image)
	if err != nil {
		return nil, err
	}

	dataNodes := make(map[string]swm_structs.DatabaseInfo, len(sql.sql.Spec.Units))
	for _, u := range sql.sql.Spec.Units {
		cc := sqlCM[u.ID]
		sqlPr = sqlPr.clone(nil)
		sqlPr.ParseData([]byte(cc.Content))

		port := sqlPr.get("mysqld::port")
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		dataNodes[u.Name] = swm_structs.DatabaseInfo{
			Ip:   sqlPr.get("mysqld::bind_address"),
			Port: p,
		}
	}

	arch := swm_structs.Type_M
	switch num := len(sql.sql.Spec.Units); {
	case num == 1:
		arch = swm_structs.Type_M
	case num == 2:
		arch = swm_structs.Type_M_SB
	case num > 2:
		arch = swm_structs.Type_M_SB_SL
	}

	topology := swm_structs.MgmPost{
		DbaasType:           arch,                      //  string   `json:"dbaas-type"`
		DbRootUser:          vars.Root.User,            //  string   `json:"db-root-user"`
		DbRootPassword:      vars.Root.Password,        //  string   `json:"db-root-password"`
		DbReplicateUser:     vars.Replication.User,     //  string   `json:"db-replicate-user"`
		DbReplicatePassword: vars.Replication.Password, //  string   `json:"db-replicate-password"`
		SwarmApiVersion:     "1.31",                    //  string   `json:"swarm-api-version,omitempty"`
		ProxyGroups:         proxyGroup,
		//	Users:               swmUsers,  //  []User   `json:"users"`
		DataNode: dataNodes, //  map[string]DatabaseInfo `json:"data-node"`
	}

	buf, err := json.Marshal(topology)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return buf, nil
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

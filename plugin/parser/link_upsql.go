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

type linkUpSQL struct {
	nameOrID string
	swm      *structs.ServiceLink
	proxy    *structs.ServiceLink
	sqls     []*structs.ServiceLink
}

func newLinkUpSQL(nameOrID string, links []*structs.ServiceLink) (linkUpSQL, error) {
	obj := linkUpSQL{
		nameOrID: nameOrID,
	}

	if len(links) < 3 {
		return obj, errors.Errorf("invalid paramaters in %s mode", SM_UPP_UPSQLs)
	}

	for i := range links {

		if links[i].Arch != (structs.Arch{}) {
			links[i].Spec.Arch = links[i].Arch
		}

		switch links[i].Spec.Image.Name {
		case "upsql":

			if obj.sqls == nil {
				obj.sqls = make([]*structs.ServiceLink, 0, 2)
			}

			obj.sqls = append(obj.sqls, links[i])

		case "upproxy":
			obj.proxy = links[i]

		case "switch_manager":
			obj.swm = links[i]

		default:
			return obj, errors.Errorf("Unsupported image %s in link %s", links[i].Spec.Image.Image(), SM_UPP_UPSQLs)
		}
	}

	if obj.proxy == nil || obj.sqls == nil || obj.swm == nil {
		return obj, errors.Errorf("the condition is not satisfied mode %s", SM_UPP_UPSQLs)
	}

	return obj, nil
}

func (lus linkUpSQL) generateLinkConfig(ctx context.Context, client kvstore.Store) (structs.ServiceLinkResponse, error) {
	resp := structs.ServiceLinkResponse{
		Links:                make([]structs.UnitLink, 0, 6),
		ReloadServicesConfig: make([]string, 2+len(lus.sqls)),
	}

	resp.ReloadServicesConfig[0] = lus.proxy.ID
	resp.ReloadServicesConfig[1] = lus.swm.ID
	for i := range lus.sqls {
		resp.ReloadServicesConfig[2+i] = lus.sqls[i].ID
	}

	{
		// sqls
		ips := make([]string, 0, len(lus.proxy.Spec.Units))
		for _, u := range lus.proxy.Spec.Units {
			if len(u.Networking) > 0 {
				ips = append(ips, u.Networking[0].IP)
			}
		}

		opts := make(map[string]map[string]interface{})
		opts[allUnitsEffect] = map[string]interface{}{"mysqld::upsql_ee_cheat_iplist": strings.Join(ips, ",")}

		for _, sql := range lus.sqls {
			if !isDesignated(lus.nameOrID, "", sql.Spec) {
				continue
			}

			ulinks, err := generateServiceLink(ctx, client, *sql.Spec, opts)
			if err != nil {
				return resp, err
			}

			for i := range ulinks {
				if isDesignated(lus.nameOrID, ulinks[i].NameOrID, lus.proxy.Spec) {
					resp.Links = append(resp.Links, ulinks[i])
				}
			}
		}
	}

	// swm
	swmCM, swmPr, err := getServiceConfigParser(ctx, client, lus.swm.Spec.ID, lus.swm.Spec.Image)
	if err != nil {
		return resp, err
	}

	swmAddr := ""
	swmc := swmCM[lus.swm.Spec.Units[0].ID]
	swmPr = swmPr.clone(nil)
	err = swmPr.ParseData([]byte(swmc.Content))
	if err != nil {
		return resp, err
	}

	{
		// proxy
		opts := make(map[string]map[string]interface{})
		// set options
		{
			ip := ""
			if len(lus.swm.Spec.Units[0].Networking) == 0 {
				return resp, errors.Errorf("unit networking is required,unit=%s", lus.swm.Spec.Units[0].Name)
			} else {
				ip = lus.swm.Spec.Units[0].Networking[0].IP
			}

			port, _ := swmPr.get("proxyport")

			opts[allUnitsEffect] = map[string]interface{}{"adm-cli::adm-svr-address": net.JoinHostPort(ip, port)}

			p, _ := swmPr.get("port")
			swmAddr = net.JoinHostPort(ip, p)
		}

		if isDesignated(lus.nameOrID, "", lus.proxy.Spec) {

			ulinks, err := generateServiceLink(ctx, client, *lus.proxy.Spec, opts)
			if err != nil {
				return resp, err
			}

			for i := range ulinks {
				if isDesignated(lus.nameOrID, ulinks[i].NameOrID, lus.proxy.Spec) {
					resp.Links = append(resp.Links, ulinks[i])
				}
			}
		}
	}

	{
		// swm
		if lus.swm != nil && isDesignated(lus.nameOrID, "", lus.swm.Spec) {
			body, err := swmInitTopology(ctx, client, lus.swm, lus.proxy, lus.sqls)
			if err != nil {
				return resp, err
			}

			for i := range lus.swm.Spec.Units {
				if !isDesignated(lus.nameOrID, lus.swm.Spec.Units[i].ID, lus.swm.Spec) {
					continue
				}

				resp.Links = append(resp.Links, structs.UnitLink{
					NameOrID:  lus.swm.Spec.Units[i].ID,
					ServiceID: lus.swm.Spec.ID,
					Commands:  swmc.Cmds[structs.RestartServiceCmd],
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

func getServiceConfigParser(ctx context.Context, kvc kvstore.Store, service string, im structs.ImageVersion) (structs.ConfigsMap, parser, error) {
	cm, err := getConfigMapFromStore(ctx, kvc, service)
	if err != nil {
		return nil, nil, err
	}

	pr, err := factoryByImage(im)
	if err != nil {
		return cm, nil, err
	}

	return cm, pr, err
}

func swmInitTopology(ctx context.Context, kvc kvstore.Store,
	swm, proxy *structs.ServiceLink,
	sqls []*structs.ServiceLink) ([]byte, error) {

	if len(sqls) == 0 || proxy == nil || swm == nil ||
		len(proxy.Spec.Units) == 0 ||
		len(swm.Spec.Units) == 0 {
		return nil, nil
	}

	proxyCM, proxyPr, err := getServiceConfigParser(ctx, kvc, proxy.Spec.ID, proxy.Spec.Image)
	if err != nil {
		return nil, err
	}

	proxyGroup := make(map[string]*swm_structs.ProxyInfo, len(proxy.Spec.Units))
	for _, u := range proxy.Spec.Units {
		cc := proxyCM[u.ID]
		proxyPr = proxyPr.clone(nil)
		proxyPr.ParseData([]byte(cc.Content))

		name, _ := proxyPr.get("upsql-proxy::proxy-name")
		addr, _ := proxyPr.get("upsql-proxy::proxy-address") // proxy-address = <proxy_ip_addr>:<proxy_data_port>
		parts := strings.SplitN(addr, ":", 2)

		proxyGroup[name] = &swm_structs.ProxyInfo{
			Id:   strconv.Itoa(int(utils.IPToUint32(parts[0]))),
			Name: name,
			Ip:   parts[0],
			Port: parts[1],
		}
	}

	dataNodesMap := make(map[string]map[string]swm_structs.DatabaseInfo, len(sqls))

	for _, sql := range sqls {
		sqlCM, sqlPr, err := getServiceConfigParser(ctx, kvc, sql.Spec.ID, sql.Spec.Image)
		if err != nil {
			return nil, err
		}

		dataNodes := make(map[string]swm_structs.DatabaseInfo, len(sql.Spec.Units))

		for _, u := range sql.Spec.Units {
			cc := sqlCM[u.ID]
			sqlPr = sqlPr.clone(nil)
			sqlPr.ParseData([]byte(cc.Content))

			port, _ := sqlPr.get("mysqld::port")
			p, err := strconv.Atoi(port)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			addr, _ := sqlPr.get("mysqld::bind_address")
			dataNodes[u.Name] = swm_structs.DatabaseInfo{
				Ip:   addr,
				Port: p,
			}
		}

		dataNodesMap[sql.ID] = dataNodes
	}

	arch := swm_structs.Type_M
	switch num := len(sqls[0].Spec.Units); {
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
		DataNode: dataNodesMap, //  map[string]map[string]DatabaseInfo `json:"data-node"`
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
		//		cmd := make([]string, len(cc.Cmds[structs.StopServiceCmd]), len(cc.Cmds[structs.StopServiceCmd])+1+len(cc.Cmds[structs.StartServiceCmd]))
		//		copy(cmd, cc.Cmds[structs.StopServiceCmd])
		//		cmd = append(cmd, "&&")
		//		cmd = append(cmd, cc.Cmds[structs.StopServiceCmd])
		ulinks = append(ulinks, structs.UnitLink{
			NameOrID:      cc.ID,
			ServiceID:     spec.ID,
			ConfigFile:    cc.ConfigFile,
			ConfigContent: cc.Content,
			Commands:      cc.Cmds[structs.RestartServiceCmd], // TODO:
		})
	}

	return ulinks, nil
}

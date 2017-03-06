package api

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden"
	"github.com/gorilla/mux"
	goctx "golang.org/x/net/context"
)

const (
	_Garden    = "garden"
	_Cluster   = "cluster"
	_tlsConfig = "tls"
)

func tlsFromContext(ctx goctx.Context, key string) (bool, *tls.Config) {
	if key != _tlsConfig {
		return false, nil
	}

	c, ok := ctx.Value(_Garden).(*context)
	if !ok || c == nil {
		return false, nil
	}

	return true, c.tlsConfig
}

func fromContext(ctx goctx.Context, key string) (bool, cluster.Cluster, *garden.Garden) {
	c, ok := ctx.Value(_Garden).(*context)
	if !ok || c == nil {
		return false, nil, nil
	}

	if key == _Garden && c.cluster != nil {

		gd, ok := c.cluster.(*garden.Garden)
		if ok {
			return true, c.cluster, gd
		} else {
			return false, c.cluster, nil
		}
	}

	return true, c.cluster, nil
}

type ctxHandler func(ctx goctx.Context, w http.ResponseWriter, r *http.Request)

var masterRoutes = map[string]map[string]ctxHandler{
	http.MethodGet: {
		"/clusters":        getClusters,
		"/clusters/{name}": getClustersByID,
		//		"/nodes":                           getAllNodes,
		//		"/nodes/{name:.*}":                 getNode,
		//		"/resources":                       getClustersResource,
		//		"/resources/{cluster:.*}":          getNodesResourceByCluster,
		//		"/tasks":                           getTasks,
		//		"/tasks/{name:.*}":                 getTask,
		//		"/ports":                           getPorts,
		//		"/networkings":                     getNetworkings,
		//		"/image/{name:.*}":                 getImage,
		//		"/services":                        getServices,
		//		"/services/{name}":                 getServicesByNameOrID,
		//		"/services/{name}/users":           getServiceUsers,
		//		"/services/{name}/topology":        hijackTopology,
		//		"/services/{name}/proxys":          hijackProxys,
		//		"/services/{name}/service_config":  getServiceServiceConfig,
		//		"/services/{name}/backup_strategy": getServiceBackupStrategy,
		//		"/services/{name}/backup_files":    getServiceBackupFiles,
		//		"/storage/san":                     getSANStoragesInfo,
		//		"/storage/san/{name:.*}":           getSANStorageInfo,
	},
	http.MethodPost: {
		// "/datacenter": postRegisterDC,
		"/clusters": postCluster,
		//		"/clusters/{name}/enable":        postEnableCluster,
		//		"/clusters/{name}/disable":       postDisableCluster,
		"/hosts": postNodes,
		//		"/clusters/nodes/{node}/enable":  postEnableNode,
		//		"/clusters/nodes/{node}/disable": postDisableNode,
		//		"/clusters/nodes/{node}/update":  updateNode,

		"/services": postService,
		//		"/services/{name:.*}/rebuild": postServiceRebuild,
		//		"/services/{name:.*}/start":   postServiceStart,
		//		"/services/{name:.*}/stop":    postServiceStop,
		//		"/services/{name:.*}/backup":  postServiceBackup,
		//		"/services/{name:.*}/scale":   postServiceScaled,

		//		"/services/{name:.*}/users": postServiceUsers,
		//		// "/services/{name:.*}/service_config/update": postServiceConfig,
		//		"/services/{name:.*}/service_config/update": postServiceConfigModify,

		//		"/services/{name:.*}/backup_strategy":         postStrategyToService,
		//		"/services/backup_strategy/{name:.*}/update":  postUpdateServiceStrategy,
		//		"/services/backup_strategy/{name:.*}/enable":  postEnableServiceStrategy,
		//		"/services/backup_strategy/{name:.*}/disable": postDisableServiceStrategy,

		//		"/units/{name:.*}/start":      postUnitStart,
		//		"/units/{name:.*}/stop":       postUnitStop,
		//		"/units/{name:.*}/backup":     postUnitBackup,
		//		"/units/{name:.*}/restore":    postUnitRestore,
		//		"/units/{name:.*}/migrate":    postUnitMigrate,
		//		"/units/{name:.*}/rebuild":    postUnitRebuild,
		//		"/units/{name:.*}/isolate":    postUnitIsolate,
		//		"/units/{name:.*}/switchback": postUnitSwitchback,

		//		"/ports":                         postImportPort,
		"/networkings/{name}/ips": postNetworking,
		//		"/networkings/{name:.*}/enable":  postEnableNetworking,
		//		"/networkings/{name:.*}/disable": postDisableNetworking,

		"/softwares/images": postImageLoad,
		//		"/image/{image:.*}/enable":   postEnableImage,
		//		"/image/{image:.*}/disable":  postDisableImage,
		//		"/image/{image:.*}/template": updateImageTemplateConfig,

		//		"/tasks/backup/callback": postBackupCallback,

		//		"/storage/san":                                   postSanStorage,
		//		"/storage/san/{name}/raid_group":                 postRGToSanStorage,
		//		"/storage/san/{name}/raid_group/{rg:.*}/enable":  postEnableRaidGroup,
		//		"/storage/san/{name}/raid_group/{rg:.*}/disable": postDisableRaidGroup,
	},

	http.MethodPut: {
		"/clusters/{name}": putClusterParams,

		"/hosts/{name}":         putNodeParam,
		"/hosts/{name}/enable":  putNodeEnable,
		"/hosts/{name}/disable": putNodeDisable,

		"/networkings/{name}/ips/enable":  putNetworkingEnable,
		"/networkings/{name}/ips/disable": putNetworkingDisable,
	},

	http.MethodDelete: {
		//		"/services/{name}":                    deleteService,
		//		"/services/{name}/users":              deleteServiceUsers,
		//		"/services/backup_strategy/{name:.*}": deleteBackupStrategy,

		"/clusters/{name}": deleteCluster,
		"/hosts/{node:.*}": deleteNode,

		//		"/networkings/{name:.*}": deleteNetworking,
		//		"/ports/{port:[0-9]+}":   deletePort,

		//		"/storage/san/{name}":                    deleteStorage,
		//		"/storage/san/{name}/raid_group/{rg:.*}": deleteRaidGroup,

		//		"/image/{image:.*}": deleteImage,
	},
}

func setupMasterRouter(r *mux.Router, context *context, enableCors bool) {

	for method, mappings := range masterRoutes {
		for route, fct := range mappings {
			logrus.WithFields(logrus.Fields{"method": method, "route": route}).Debug("Registering HTTP route")

			localRoute := route
			localFct := fct

			wrap := func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()

				if enableCors {
					writeCorsHeaders(w, r)
				}

				context.apiVersion = mux.Vars(r)["version"]
				ctx := goctx.WithValue(goctx.Background(), _Garden, context)

				localFct(ctx, w, r)

				logrus.WithFields(logrus.Fields{"method": r.Method,
					"uri":   r.RequestURI,
					"since": time.Since(start).String()}).Debug("HTTP request received")
			}

			localMethod := method

			r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			r.Path(localRoute).Methods(localMethod).HandlerFunc(DebugRequestMiddleware(wrap))

			if enableCors {
				optionsMethod := "OPTIONS"
				optionsFct := optionsHandler

				wrap := func(w http.ResponseWriter, r *http.Request) {
					logrus.WithFields(logrus.Fields{"method": optionsMethod, "uri": r.RequestURI}).
						Debug("HTTP request received")
					if enableCors {
						writeCorsHeaders(w, r)
					}
					context.apiVersion = mux.Vars(r)["version"]
					optionsFct(context, w, r)
				}

				r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).
					Methods(optionsMethod).HandlerFunc(wrap)
				r.Path(localRoute).Methods(optionsMethod).
					HandlerFunc(wrap)
			}

		}
	}
}

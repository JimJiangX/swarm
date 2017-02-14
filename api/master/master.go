package api

import (
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

const (
	_Garden  = "garden"
	_Cluster = "context"
)

func fromContext(ctx context.Context, key string) (bool, cluster.Cluster, *garden.Garden) {
	c, ok := ctx.Value(_Garden).(cluster.Cluster)
	if !ok {
		return false, nil, nil
	}

	if key == _Cluster {
		return true, c, nil
	}

	gd, ok := c.(*garden.Garden)
	if !ok {
		return false, c, nil
	}

	return true, c, gd
}

type handler func(ctx context.Context, w http.ResponseWriter, r *http.Request)

var routes = map[string]map[string]handler{
	"GET": {
	//		"/clusters":                        getClusters,
	//		"/clusters/{name}":                 getClustersByNameOrID,
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
	"POST": {
	//		"/datacenter":                    postDatacenter,
	//		"/clusters":                      postCluster,
	//		"/clusters/{name}/update":        postUpdateClusterParams,
	//		"/clusters/{name}/enable":        postEnableCluster,
	//		"/clusters/{name}/disable":       postDisableCluster,
	//		"/clusters/{name}/nodes":         postNodes,
	//		"/clusters/nodes/{node}/enable":  postEnableNode,
	//		"/clusters/nodes/{node}/disable": postDisableNode,
	//		"/clusters/nodes/{node}/update":  updateNode,

	//		"/services":                   postService,
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
	//		"/networkings":                   postNetworking,
	//		"/networkings/{name:.*}/enable":  postEnableNetworking,
	//		"/networkings/{name:.*}/disable": postDisableNetworking,

	//		"/image/load":                postImageLoad,
	//		"/image/{image:.*}/enable":   postEnableImage,
	//		"/image/{image:.*}/disable":  postDisableImage,
	//		"/image/{image:.*}/template": updateImageTemplateConfig,

	//		"/tasks/backup/callback": postBackupCallback,

	//		"/storage/san":                                   postSanStorage,
	//		"/storage/san/{name}/raid_group":                 postRGToSanStorage,
	//		"/storage/san/{name}/raid_group/{rg:.*}/enable":  postEnableRaidGroup,
	//		"/storage/san/{name}/raid_group/{rg:.*}/disable": postDisableRaidGroup,
	},
	"DELETE": {
	//		"/services/{name}":                    deleteService,
	//		"/services/{name}/users":              deleteServiceUsers,
	//		"/services/backup_strategy/{name:.*}": deleteBackupStrategy,

	//		"/clusters/{name}":          deleteCluster,
	//		"/clusters/nodes/{node:.*}": deleteNode,

	//		"/networkings/{name:.*}": deleteNetworking,
	//		"/ports/{port:[0-9]+}":   deletePort,

	//		"/storage/san/{name}":                    deleteStorage,
	//		"/storage/san/{name}/raid_group/{rg:.*}": deleteRaidGroup,

	//		"/image/{image:.*}": deleteImage,
	},
}

func SetupMasterRouter(r *mux.Router, c cluster.Cluster, enableCors bool) {

	for method, mappings := range routes {
		for route, fct := range mappings {
			logrus.WithFields(logrus.Fields{"method": method, "route": route}).Debug("Registering HTTP route")

			localRoute := route
			localFct := fct

			wrap := func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()

				if enableCors {
					writeCorsHeaders(w, r)
				}

				ctx := context.WithValue(context.Background(), _Garden, c)
				localFct(ctx, w, r)

				logrus.WithFields(logrus.Fields{"method": r.Method,
					"uri":   r.RequestURI,
					"since": time.Since(start).String()}).Debug("HTTP request received")
			}
			localMethod := method

			r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			r.Path(localRoute).Methods(localMethod).
				HandlerFunc(DebugRequestMiddleware(wrap))

			if enableCors {
				optionsMethod := "OPTIONS"
				optionsFct := optionsHandler

				wrap := func(w http.ResponseWriter, r *http.Request) {
					logrus.WithFields(logrus.Fields{"method": optionsMethod, "uri": r.RequestURI}).
						Debug("HTTP request received")
					if enableCors {
						writeCorsHeaders(w, r)
					}

					optionsFct(w, r)
				}

				r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).
					Methods(optionsMethod).HandlerFunc(wrap)
				r.Path(localRoute).Methods(optionsMethod).
					HandlerFunc(wrap)
			}
		}
	}
}

func optionsHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func writeCorsHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	w.Header().Add("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, OPTIONS, HEAD")
}
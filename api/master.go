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
	_Garden         = "garden"
	_ClusterContext = "cluster"
	_tlsConfig      = "tls"
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

		return ok, c.cluster, gd
	}

	return true, c.cluster, nil
}

type ctxHandler func(ctx goctx.Context, w http.ResponseWriter, r *http.Request)

var masterRoutes = map[string]map[string]ctxHandler{
	http.MethodGet: {
		"/nfs_backups/space": getNFSSPace,

		"/clusters":        getClusters,
		"/clusters/{name}": getClustersByID,
		"/hosts":           getAllNodes,
		"/hosts/{name:.*}": getNode,

		"/networkings":        listNetworkings,
		"/networkings/{name}": getNetworking,

		"/tasks":        getTasks,
		"/tasks/{name}": getTask,

		"/softwares/images":           listImages,
		"/softwares/images/supported": getSupportImages,

		"/services":        getServices,
		"/services/{name}": getServicesByNameOrID,

		"/storage/san":           getSANStoragesInfo,
		"/storage/san/{name:.*}": getSANStorageInfo,
	},
	http.MethodPost: {
		"/clusters": postCluster,

		"/hosts": postNodes,

		"/services":      postService,
		"/services/link": postServiceLink,

		"/services/{name}/scale":         postServiceScaled,
		"/services/{name}/update":        postServiceUpdate,
		"/services/{name}/image/update":  postServiceVersionUpdate,
		"/services/{name}/start":         postServiceStart,
		"/services/{name}/stop":          postServiceStop,
		"/services/{name}/config/update": postServiceUpdateConfigs,
		"/services/{name}/exec":          postServiceExec,
		"/services/{name:.*}/backup":     postServiceBackup,
		//		"/services/{name:.*}/rebuild": postServiceRebuild,

		//		"/units/{name:.*}/migrate":    postUnitMigrate,
		//		"/units/{name:.*}/rebuild":    postUnitRebuild,

		"/networkings/{name}/ips": postNetworking,

		"/softwares/images": postImageLoad,

		"/tasks/backup/callback": postBackupCallback,

		"/storage/san":                        postSanStorage,
		"/storage/san/{name}/raid_group/{rg}": postRGToSanStorage,
	},

	http.MethodPut: {
		"/clusters/{name}": putClusterParams,

		"/hosts/{name}":         putNodeParam,
		"/hosts/{name}/enable":  putNodeEnable,
		"/hosts/{name}/disable": putNodeDisable,

		"/networkings/{name}/ips/enable":  putNetworkingEnable,
		"/networkings/{name}/ips/disable": putNetworkingDisable,

		"/storage/san/{name}/raid_group/{rg:.*}/enable":  postEnableRaidGroup,
		"/storage/san/{name}/raid_group/{rg:.*}/disable": postDisableRaidGroup,
	},

	http.MethodDelete: {
		"/services/{name}": deleteService,

		"/clusters/{name}": deleteCluster,
		"/hosts/{node:.*}": deleteNode,

		"/networkings/{name}/ips": deleteNetworking,

		"/storage/san/{name}":                    deleteStorage,
		"/storage/san/{name}/raid_group/{rg:.*}": deleteRaidGroup,

		"/softwares/images/{image}": deleteImage,
	},
}

func setupMasterRouter(r *mux.Router, context *context, debug, enableCors bool) {
	wrap := func(fct ctxHandler) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			if enableCors {
				writeCorsHeaders(w, r)
			}

			context.apiVersion = mux.Vars(r)["version"]
			ctx := goctx.Background()

			if wait := intValueOrZero(r, "wait"); wait > 0 {
				ctx, _ = goctx.WithTimeout(ctx, time.Duration(wait)*time.Second)
			}

			ctx = goctx.WithValue(ctx, _Garden, context)

			fct(ctx, w, r)

			logrus.WithFields(logrus.Fields{"method": r.Method,
				"uri":   r.RequestURI,
				"since": time.Since(start).String()}).Info("HTTP request received")
		}
	}

	if debug {
		r.HandleFunc("/v{version:[0-9]+.[0-9]+}"+"/units/{name}/proxy/{proxy:.*}", DebugRequestMiddleware(wrap(proxySpecialLogic)))
	} else {
		r.HandleFunc("/v{version:[0-9]+.[0-9]+}"+"/units/{name}/proxy/{proxy:.*}", wrap(proxySpecialLogic))
	}
	for method, mappings := range masterRoutes {
		for route, fct := range mappings {
			logrus.WithFields(logrus.Fields{"method": method, "route": route}).Debug("Registering HTTP route")

			localRoute := route
			localMethod := method
			localFct := wrap(fct)

			if debug {
				r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(DebugRequestMiddleware(localFct))
				r.Path(localRoute).Methods(localMethod).HandlerFunc(DebugRequestMiddleware(localFct))
			} else {
				r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(localFct)
				r.Path(localRoute).Methods(localMethod).HandlerFunc(localFct)
			}

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

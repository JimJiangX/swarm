package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/swarm/cluster/swarm"
	"github.com/gorilla/mux"
	goctx "golang.org/x/net/context"
)

const (
	_Gardener = "gardener"
	_Context  = "gardener context"
)

var enableGardener = false

// EnableGardener set enableGardener
func EnableGardener(enable bool) {
	enableGardener = enable
}

func isGardenerEnable() bool {
	return enableGardener
}

func fromContext(ctx goctx.Context, key string) (bool, *context, *swarm.Gardener) {
	c, ok := ctx.Value(_Gardener).(*context)
	if !ok {
		return false, nil, nil
	}

	if key == _Context {
		return true, c, nil
	}

	gd, ok := c.cluster.(*swarm.Gardener)
	if !ok {
		return false, c, nil
	}

	return true, c, gd
}

type ctxHandler func(ctx goctx.Context, w http.ResponseWriter, r *http.Request)

var masterRoutes = map[string]map[string]ctxHandler{
	"GET": {
		"/clusters":                        getClusters,
		"/clusters/{name}":                 getClustersByNameOrID,
		"/nodes":                           getAllNodes,
		"/nodes/{name:.*}":                 getNode,
		"/resources":                       getClustersResource,
		"/resources/{cluster:.*}":          getNodesResourceByCluster,
		"/tasks":                           getTasks,
		"/tasks/{name:.*}":                 getTask,
		"/ports":                           getPorts,
		"/networkings":                     getNetworkings,
		"/image/{name:.*}":                 getImage,
		"/services":                        getServices,
		"/services/{name}":                 getServicesByNameOrID,
		"/services/{name}/users":           getServiceUsers,
		"/services/{name}/topology":        hijackTopology,
		"/services/{name}/proxys":          hijackProxys,
		"/services/{name}/service_config":  getServiceServiceConfig,
		"/services/{name}/backup_strategy": getServiceBackupStrategy,
		"/services/{name}/backup_files":    getServiceBackupFiles,
		"/storage/san":                     getSANStoragesInfo,
		"/storage/san/{name:.*}":           getSANStorageInfo,
	},
	"POST": {
		"/datacenter":                    postDatacenter,
		"/clusters":                      postCluster,
		"/clusters/{name}/update":        postUpdateClusterParams,
		"/clusters/{name}/enable":        postEnableCluster,
		"/clusters/{name}/disable":       postDisableCluster,
		"/clusters/{name}/nodes":         postNodes,
		"/clusters/nodes/{node}/enable":  postEnableNode,
		"/clusters/nodes/{node}/disable": postDisableNode,
		"/clusters/nodes/{node}/update":  updateNode,

		"/services":                   postService,
		"/services/{name:.*}/rebuild": postServiceRebuild,
		"/services/{name:.*}/start":   postServiceStart,
		"/services/{name:.*}/stop":    postServiceStop,
		"/services/{name:.*}/backup":  postServiceBackup,
		"/services/{name:.*}/scale":   postServiceScaled,

		"/services/{name:.*}/users": postServiceUsers,
		// "/services/{name:.*}/service_config/update": postServiceConfig,
		"/services/{name:.*}/service_config/update": postServiceConfigModify,

		"/services/{name:.*}/backup_strategy":         postStrategyToService,
		"/services/backup_strategy/{name:.*}/update":  postUpdateServiceStrategy,
		"/services/backup_strategy/{name:.*}/enable":  postEnableServiceStrategy,
		"/services/backup_strategy/{name:.*}/disable": postDisableServiceStrategy,

		"/units/{name:.*}/start":      postUnitStart,
		"/units/{name:.*}/stop":       postUnitStop,
		"/units/{name:.*}/backup":     postUnitBackup,
		"/units/{name:.*}/restore":    postUnitRestore,
		"/units/{name:.*}/migrate":    postUnitMigrate,
		"/units/{name:.*}/rebuild":    postUnitRebuild,
		"/units/{name:.*}/isolate":    postUnitIsolate,
		"/units/{name:.*}/switchback": postUnitSwitchback,

		"/ports":                         postImportPort,
		"/networkings":                   postNetworking,
		"/networkings/{name:.*}/enable":  postEnableNetworking,
		"/networkings/{name:.*}/disable": postDisableNetworking,

		"/image/load":                postImageLoad,
		"/image/{image:.*}/enable":   postEnableImage,
		"/image/{image:.*}/disable":  postDisableImage,
		"/image/{image:.*}/template": updateImageTemplateConfig,

		"/tasks/backup/callback": postBackupCallback,

		"/storage/san":                                   postSanStorage,
		"/storage/san/{name}/raid_group":                 postRGToSanStorage,
		"/storage/san/{name}/raid_group/{rg:.*}/enable":  postEnableRaidGroup,
		"/storage/san/{name}/raid_group/{rg:.*}/disable": postDisableRaidGroup,
	},
	"DELETE": {
		"/services/{name}":                    deleteService,
		"/services/{name}/users":              deleteServiceUsers,
		"/services/backup_strategy/{name:.*}": deleteBackupStrategy,

		"/clusters/{name}":          deleteCluster,
		"/clusters/nodes/{node:.*}": deleteNode,

		"/networkings/{name:.*}": deleteNetworking,
		"/ports/{port:[0-9]+}":   deletePort,

		"/storage/san/{name}":                    deleteStorage,
		"/storage/san/{name}/raid_group/{rg:.*}": deleteRaidGroup,

		"/image/{image:.*}": deleteImage,
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
				if logrus.GetLevel() == logrus.DebugLevel {
					DebugRequestMiddleware(r)
				}

				context.apiVersion = mux.Vars(r)["version"]
				ctx := goctx.WithValue(goctx.Background(), _Gardener, context)
				localFct(ctx, w, r)

				logrus.WithFields(logrus.Fields{"method": r.Method,
					"uri":   r.RequestURI,
					"since": time.Since(start).String()}).Debug("HTTP request received")
			}
			localMethod := method

			r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			r.Path(localRoute).Methods(localMethod).HandlerFunc(wrap)

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

// DebugRequestMiddleware dumps the request to logger
func DebugRequestMiddleware(r *http.Request) error {
	if (r.Method != "POST" && r.Method != "PUT") ||
		r.ContentLength == 0 || r.Body == nil {
		logrus.Warnf("Request Method %s,ContentLength %d", r.Method, r.ContentLength)
		return nil
	}

	maxBodySize := 4096 // 4KB
	if r.ContentLength > int64(maxBodySize) {
		logrus.Warnf("ContentLength Too Large %d>%d", r.ContentLength, maxBodySize)
		return nil
	}

	body := r.Body
	bufReader := bufio.NewReaderSize(body, maxBodySize)
	r.Body = ioutils.NewReadCloserWrapper(bufReader, func() error { return body.Close() })

	b, err := bufReader.Peek(maxBodySize)
	if err != io.EOF {
		// either there was an error reading, or the buffer is full (in which case the request is too large)
		logrus.Debug(err, " either there was an error reading, or the buffer is full (in which case the request is too large)")
		return err
	}

	var postForm map[string]interface{}
	if err := json.Unmarshal(b, &postForm); err == nil {
		if _, exists := postForm["password"]; exists {
			postForm["password"] = "*****"
		}
		formStr, errMarshal := json.Marshal(postForm)
		if errMarshal == nil {
			logrus.Debugf("form data: %s", string(formStr))
		} else {
			logrus.Debugf("form data: %q", postForm)
		}
	} else {
		logrus.Debugf("'%s'", b)
	}

	return err
}

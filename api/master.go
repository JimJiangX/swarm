package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"

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

func EnableGardener(enable bool) {
	enableGardener = enable
}

func IsGardenerEnable() bool {
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
		"/clusters/{name:.*}":              getClustersByNameOrID,
		"/clusters/nodes/{name:.*}":        getNode,
		"/clusters/resources":              getNodesResourceByCluster,
		"/clusters/{name}/nodes/{node:.*}": getNodeResourceByNameOrID,
		"/tasks":           getTasks,
		"/tasks/{name:.*}": getTask,
		"/ports":           getPorts,
		"/networkings":     getNetworkings,
		"/image/{name:.*}": getImage,
	},
	"POST": {
		"/clusters":                      postCluster,
		"/clusters/{name}/update":        postUpdateClusterParams,
		"/clusters/{name}/enable":        postEnableCluster,
		"/clusters/{name}/disable":       postDisableCluster,
		"/clusters/{name}/nodes":         postNodes,
		"/clusters/nodes/{node}/enable":  postEnableNode,
		"/clusters/nodes/{node}/disable": postDisableNode,
		"/clusters/nodes/{node}/update":  updateNode,

		"/services":                    postService,
		"/services/{name:.*}/start":    postServiceStart,
		"/services/{name:.*}/stop":     postServiceStop,
		"/services/{name:.*}/backup":   postServiceBackup,
		"/services/{name:.*}/recover":  postServiceRecover,
		"/services/{name:.*}/recreate": postServiceRecreate,

		"/services/{name:.*}/backup_strategy":         postStrategyToService,
		"/services/backup_strategy/{name:.*}/update":  postUpdateServiceStrategy,
		"/services/backup_strategy/{name:.*}/enable":  postEnableServiceStrategy,
		"/services/backup_strategy/{name:.*}/disable": postDisableServiceStrategy,

		"/units/{unit:.*}/start":      postUnitStart,
		"/units/{unit:.*}/stop":       postUnitStop,
		"/units/{unit:.*}/backup":     postUnitBackup,
		"/units/{unit:.*}/recover":    postUnitRecover,
		"/units/{unit:.*}/migrate":    postUnitMigrate,
		"/units/{unit:.*}/rebuild":    postUnitRebuild,
		"/units/{unit:.*}/isolate":    postUnitIsolate,
		"/units/{unit:.*}/switchback": postUnitSwitchback,

		"/ports":                         postImportPort,
		"/networkings":                   postNetworking,
		"/networkings/{name:.*}/enable":  postEnableNetworking,
		"/networkings/{name:.*}/disable": postDisableNetworking,

		"/image/load":                postImageLoad,
		"/image/{image:.*}/enable":   postEnableImage,
		"/image/{image:.*}/disable":  postDisableImage,
		"/image/{image:.*}/template": updateImageTemplateConfig,

		"/storage/nas":                                       postNasStorage,
		"/storage/san":                                       postSanStorage,
		"/storage/san/{name}/raid_group":                     postRGToSanStorage,
		"/storage/san/{name}/raid_group/{rg:[0-9]+}/enable":  postEnableRaidGroup,
		"/storage/san/{name}/raid_group/{rg:[0-9]+}/disable": postDisableRaidGroup,

		"/tasks/backup/callback": postBackupCallback,
	},
	"DELETE": {
		"/services/{name}":                    deleteService,
		"/services/backup_strategy/{name:.*}": deleteBackupStrategy,

		"/clusters/{name}":          deleteCluster,
		"/clusters/nodes/{node:.*}": deleteNode,

		"/networkings/{name:.*}": deleteNetworking,
		"/ports/{port:[0-9]+}":   deletePort,

		"/storage/san/{name}":                        deleteStorage,
		"/storage/san/{name}/raid_group/{rg:[0-9]+}": deleteRaidGroup,

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
				logrus.WithFields(logrus.Fields{"method": r.Method, "uri": r.RequestURI}).Debug("HTTP request received")
				if enableCors {
					writeCorsHeaders(w, r)
				}
				if logrus.GetLevel() == logrus.DebugLevel {
					DebugRequestMiddleware(r)
				}
				context.apiVersion = mux.Vars(r)["version"]
				ctx := goctx.WithValue(goctx.Background(), _Gardener, context)
				localFct(ctx, w, r)
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
	if r.Method != "POST" {
		return nil
	}

	maxBodySize := 20 << 1 // 1MB
	if r.ContentLength > int64(maxBodySize) {
		return nil
	}

	body := r.Body
	bufReader := bufio.NewReaderSize(body, maxBodySize)
	r.Body = ioutils.NewReadCloserWrapper(bufReader, func() error { return body.Close() })

	b, err := bufReader.Peek(maxBodySize)
	if err != io.EOF {
		// either there was an error reading, or the buffer is full (in which case the request is too large)
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
	}

	return err
}

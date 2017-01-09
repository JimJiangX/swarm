package parser

import (
	"context"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/gorilla/mux"
)

type _Context struct {
	apiVersion string
	client     kvstore.Client
	context    context.Context
}

func NewRouter(c kvstore.Client) *mux.Router {
	type handler func(ctx *_Context, w http.ResponseWriter, r *http.Request)

	ctx := &_Context{client: c}

	var routes = map[string]map[string]handler{
		"GET": {
			"/image/requirement": getImageRequirement,
		},
	}

	r := mux.NewRouter()
	for method, mappings := range routes {
		for route, fct := range mappings {
			logrus.WithFields(logrus.Fields{"method": method, "route": route}).Debug("Registering HTTP route")

			localRoute := route
			localFct := fct

			wrap := func(w http.ResponseWriter, r *http.Request) {
				logrus.WithFields(logrus.Fields{"method": r.Method, "uri": r.RequestURI}).Debug("HTTP request received")

				ctx.context = r.Context()
				ctx.apiVersion = mux.Vars(r)["version"]
				timeout := mux.Vars(r)["timeout"]

				if timeout != "" {
					d, err := time.ParseDuration(timeout)
					if err != nil {
						logrus.WithError(err).Warnf("invalid timeout:%s", timeout)
					} else {
						_ctx, cancel := context.WithTimeout(ctx.context, d)
						ctx.context = _ctx
						defer cancel()
					}
				}

				localFct(ctx, w, r)
			}
			localMethod := method

			r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			r.Path(localRoute).Methods(localMethod).HandlerFunc(wrap)
		}
	}

	return r
}

func getImageRequirement(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

func getConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

func getConfig(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

func getCommand(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

func postTemplate(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

func generateConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

func generateCommands(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

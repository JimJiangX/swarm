package api

import (
	"net/http"

	goctx "golang.org/x/net/context"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster/swarm"
	"github.com/gorilla/mux"
)

const (
	enableMaster = true
	Gardener     = "gardener"
)

func fromContext(ctx goctx.Context) (bool, *context, *swarm.Region) {
	c, ok := ctx.Value(Gardener).(*context)
	if !ok {
		return false, nil, nil
	}

	r, ok := c.cluster.(*swarm.Region)

	if !ok {
		return false, c, nil
	}

	return true, c, r
}

type ctxHandler func(ctx goctx.Context, w http.ResponseWriter, r *http.Request)

var masterRoutes = map[string]map[string]ctxHandler{
	"POST": {
		"/cluster": postCluster,
	},
}

func setupMasterRouter(r *mux.Router, context *context, enableCors bool) {

	for method, mappings := range masterRoutes {
		for route, fct := range mappings {
			log.WithFields(log.Fields{"method": method, "route": route}).Debug("Registering HTTP route")

			localRoute := route
			localFct := fct

			wrap := func(w http.ResponseWriter, r *http.Request) {
				log.WithFields(log.Fields{"method": r.Method, "uri": r.RequestURI}).Debug("HTTP request received")
				if enableCors {
					writeCorsHeaders(w, r)
				}
				context.apiVersion = mux.Vars(r)["version"]
				ctx := goctx.WithValue(goctx.TODO(), Gardener, context)
				localFct(ctx, w, r)
			}
			localMethod := method

			r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			r.Path(localRoute).Methods(localMethod).HandlerFunc(wrap)

			if enableCors {
				optionsMethod := "OPTIONS"
				optionsFct := optionsHandler

				wrap := func(w http.ResponseWriter, r *http.Request) {
					log.WithFields(log.Fields{"method": optionsMethod, "uri": r.RequestURI}).
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

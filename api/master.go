package api

import (
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster/swarm"
	"github.com/gorilla/mux"
)

// Master router context, used by handlers.
type mcontext struct {
	context
	region *swarm.Region
}

type mhandler func(c *mcontext, w http.ResponseWriter, r *http.Request)

var mroutes = map[string]map[string]mhandler{
	"POST": {
		"/cluster": postCluster,
	},
}

func setupMasterRouter(r *mux.Router, context *mcontext, enableCors bool) {
	for method, mappings := range mroutes {
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
				localFct(context, w, r)
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
					optionsFct(&context.context, w, r)
				}

				r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).
					Methods(optionsMethod).HandlerFunc(wrap)
				r.Path(localRoute).Methods(optionsMethod).
					HandlerFunc(wrap)
			}

		}
	}
}

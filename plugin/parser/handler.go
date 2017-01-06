package parser

import (
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	type handler func(w http.ResponseWriter, r *http.Request)

	var routes = map[string]map[string]handler{
		"GET": {
			"/_ping": nil,
		},
	}

	r := mux.NewRouter()
	for method, mappings := range routes {
		for route, fct := range mappings {
			logrus.WithFields(logrus.Fields{"method": method, "route": route}).Debug("Registering HTTP route")
			r.Path("/v{version:[0-9]+.[0-9]+}" + route).Methods(method).HandlerFunc(fct)
			r.Path(route).Methods(method).HandlerFunc(fct)
		}
	}

	return r
}

func getImageRequirement(w http.ResponseWriter, r *http.Request) {

}

func getConfigs(w http.ResponseWriter, r *http.Request) {

}

func getConfig(w http.ResponseWriter, r *http.Request) {

}

func getCommand(w http.ResponseWriter, r *http.Request) {

}

func postTemplate(w http.ResponseWriter, r *http.Request) {

}

func generateConfigs(w http.ResponseWriter, r *http.Request) {

}

func generateCommands(w http.ResponseWriter, r *http.Request) {

}

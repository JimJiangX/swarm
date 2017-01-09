package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
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
	req := structs.ConfigTemplate{}

	value, err := ioutil.ReadAll(r.Body)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(value, &req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	parser := factory(req.Name, req.Version)
	if parser == nil {
		httpError(w, fmt.Errorf(""), http.StatusNotImplemented)
		return
	}

	err = parser.ParseData(req.Context)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	key := filepath.Join(imageKey, req.Name, req.Version)
	err = ctx.client.PutKV(key, value)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func generateConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	req := structs.ServiceDesc{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	key := filepath.Join(imageKey, req.Name, req.Version)
	pair, err := ctx.client.GetKV(key)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	t := structs.ConfigTemplate{}
	err = json.Unmarshal(pair.Value, &t)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	resp := make(structs.ConfigsMap, len(req.Units))

	for i := range req.Units {
		parser := factory(req.Name, req.Version)
		if parser == nil {
			httpError(w, fmt.Errorf(""), http.StatusNotImplemented)
			return
		}

		err = parser.ParseData(t.Context)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		err := parser.GenerateConfig(req.Units[i].ID, req)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		context, err := parser.Marshal()
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		cmds, err := parser.GenerateCommands(req.Units[i].ID, req)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		r, err := parser.HealthCheck(req.Units[i].ID, req)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		resp[req.Units[i].ID] = structs.ConfigCmds{
			ID:           req.Units[i].ID,
			Path:         t.Path,
			Context:      string(context),
			Cmds:         cmds,
			Timestamp:    time.Now().Unix(),
			Registration: r,
		}
	}

	buf := bytes.NewBuffer(nil)
	for id, val := range resp {
		err := json.NewEncoder(buf).Encode(val)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		key := filepath.Join(configKey, req.ID, id)

		err = ctx.client.PutKV(key, buf.Bytes())
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}

	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	return
}

func updateConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

func generateCommands(ctx *_Context, w http.ResponseWriter, r *http.Request) {

}

// Emit an HTTP error and log it.
func httpError(w http.ResponseWriter, err error, status int) {
	if err != nil {
		logrus.WithField("status", status).Errorf("HTTP error: %+v", err)
		http.Error(w, err.Error(), status)
		return
	}

	http.Error(w, "", status)
}

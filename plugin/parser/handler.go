package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/gorilla/mux"
	"github.com/hashicorp/consul/api"
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
			"/image/support":                  isImageSupport,
			"/image/requirement":              getImageRequirement,
			"/configs/{service:.*}":           getConfigs,
			"/configs/{service:.*}/{unit:.*}": getConfig,
			"/commands/{service:.*}":          getCommands,
		},
		"POST": {
			"/configs":        generateConfigs,
			"/image/template": postTemplate,
		},
		"PUT": {
			"/configs/{service:.*}": updateConfigs,
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

				localFct(ctx, w, r)
			}
			localMethod := method

			r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			r.Path(localRoute).Methods(localMethod).HandlerFunc(wrap)
		}
	}

	return r
}

func isImageSupport(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	version := r.FormValue("version")

	_, err := factory(name, version)
	if err != nil {
		httpError(w, err, http.StatusNotImplemented)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getImageRequirement(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	version := r.FormValue("version")

	parser, err := factory(name, version)
	if err != nil {
		httpError(w, err, http.StatusNotImplemented)
		return
	}

	resp := parser.Requirement()

	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return
}

func getConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	service := mux.Vars(r)["service"]
	key := strings.Join([]string{configKey, service}, "/")

	pairs, err := ctx.client.ListKV(key)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	cm, err := parseListToConfigs(pairs)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	err = json.NewEncoder(w).Encode(cm)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return

}

func getConfig(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	service := mux.Vars(r)["service"]
	unit := mux.Vars(r)["unit"]
	key := strings.Join([]string{configKey, service, unit}, "/")

	val, err := ctx.client.GetKV(key)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(val.Value)

	return
}

func getCommands(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	service := mux.Vars(r)["service"]
	key := strings.Join([]string{configKey, service}, "/")

	pairs, err := ctx.client.ListKV(key)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	cm, err := parseListToConfigs(pairs)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	resp := make(structs.Commands, len(cm))

	for _, c := range cm {
		resp[c.ID] = c.Cmds
	}

	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return
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

	parser, err := factory(req.Name, req.Version)
	if err != nil {
		httpError(w, err, http.StatusNotImplemented)
		return
	}

	err = parser.ParseData(req.Context)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	key := strings.Join([]string{imageKey, req.Name, req.Version}, "/")
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

	key := strings.Join([]string{imageKey, req.Name, req.Version}, "/")
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
		parser, err := factory(req.Name, req.Version)
		if err != nil {
			httpError(w, err, http.StatusNotImplemented)
			return
		}

		err = parser.ParseData(t.Context)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		err = parser.GenerateConfig(req.Units[i].ID, req)
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
			Name:         req.Name,
			Version:      req.Version,
			Path:         t.Path,
			Context:      string(context),
			Cmds:         cmds,
			Timestamp:    time.Now().Unix(),
			Registration: r,
		}
	}

	err = putConfigsToKV(ctx.client, req.ID, resp)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
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
	var (
		req     structs.ConfigsMap
		service = mux.Vars(r)["service"]
	)

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	pairs := make(api.KVPairs, len(req))

	switch len(req) {
	case 0:
		httpError(w, fmt.Errorf(""), http.StatusBadRequest)
		return
	case 1:
		for _, c := range req {
			key := strings.Join([]string{configKey, service, c.ID}, "/")
			pair, err := ctx.client.GetKV(key)
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}
			pairs[0] = pair
		}
	default:
		key := strings.Join([]string{configKey, service}, "/")
		pairs, err = ctx.client.ListKV(key)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}

	configs, err := parseListToConfigs(pairs)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	out := make(structs.ConfigsMap, len(configs))

	for id, u := range req {
		c, exist := configs[id]
		if !exist {
			if err != nil {
				httpError(w, fmt.Errorf(""), http.StatusInternalServerError)
				return
			}
		}

		if u.ID != "" && c.ID != u.ID {
			c.ID = u.ID
		}

		if u.Path != "" && c.Path != u.Path {
			c.Path = u.Path
		}

		if u.Context != "" && c.Context != u.Context {
			parser, err := factory(c.Name, c.Version)
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}

			err = parser.ParseData([]byte(u.Context))
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}

			c.Context = u.Context
		}

		c.Timestamp = time.Now().Unix()

		out[id] = c
	}

	err = putConfigsToKV(ctx.client, service, out)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	return
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

func parseListToConfigs(pairs api.KVPairs) (structs.ConfigsMap, error) {
	cm := make(structs.ConfigsMap, len(pairs))

	for i := range pairs {
		c := structs.ConfigCmds{}
		err := json.Unmarshal(pairs[i].Value, &c)
		if err != nil {
			return nil, err
		}

		cm[c.ID] = c
	}

	return cm, nil
}

func putConfigsToKV(client kvstore.Client, prefix string, configs structs.ConfigsMap) error {
	buf := bytes.NewBuffer(nil)

	for id, val := range configs {
		buf.Reset()

		err := json.NewEncoder(buf).Encode(val)
		if err != nil {
			return err
		}

		key := strings.Join([]string{configKey, prefix, id}, "/")
		err = client.PutKV(key, buf.Bytes())
		if err != nil {
			return err
		}
	}

	return nil
}

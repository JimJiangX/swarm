package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/plugin/parser/compose"
	"github.com/gorilla/mux"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type _Context struct {
	apiVersion string
	client     kvstore.Client
	context    context.Context

	mgmIP   string
	mgmPort int
}

func NewRouter(c kvstore.Client, ip string, port int) *mux.Router {
	type handler func(ctx *_Context, w http.ResponseWriter, r *http.Request)

	ctx := &_Context{
		client:  c,
		mgmIP:   ip,
		mgmPort: port,
	}

	var routes = map[string]map[string]handler{
		"GET": {
			"/image/support": getSupportImageVersion,
			//"/image/requirement":              getImageRequirement,
			"/configs/{service:.*}":           getConfigs,
			"/configs/{service:.*}/{unit:.*}": getConfig,
			"/commands/{service:.*}":          getCommands,
		},
		"POST": {
			"/configs":        generateConfigs,
			"/image/template": postTemplate,
		},
		"PUT": {
			"/configs/{service:.*}":       updateConfigs,
			"/services/{service}/compose": composeService,
			"/services/link":              linkServices,
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

func getSupportImageVersion(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	out := make([]structs.ImageVersion, 0, 10)

	for key := range images {
		out = append(out, key)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(out)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	return
}

//func getImageRequirement(ctx *_Context, w http.ResponseWriter, r *http.Request) {
//	if err := r.ParseForm(); err != nil {
//		httpError(w, err, http.StatusBadRequest)
//		return
//	}

//	image := r.FormValue("image")

//	parts := strings.SplitN(image, ":", 2)
//	var v string
//	if len(parts) == 2 {
//		v = parts[1]
//	}

//	parser, err := factory(parts[0], v)
//	if err != nil {
//		httpError(w, err, http.StatusNotImplemented)
//		return
//	}

//	resp := parser.Requirement()

//	err = json.NewEncoder(w).Encode(resp)
//	if err != nil {
//		httpError(w, err, http.StatusInternalServerError)
//		return
//	}
//	w.Header().Set("Content-Type", "application/json")
//	w.WriteHeader(http.StatusOK)
//	return
//}

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

	// structs.ConfigCmds,encode by JSON
	pair, err := ctx.client.GetKV(key)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(pair.Value)

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

//func checkImage(ctx *_Context, w http.ResponseWriter, r *http.Request) {
//	t := structs.ConfigTemplate{}

//	err := json.NewDecoder(r.Body).Decode(&t)
//	if err != nil {
//		httpError(w, err, http.StatusBadRequest)
//		return
//	}

//	parser, err := factory(t.Name, t.Version)
//	if err != nil {
//		httpError(w, err, http.StatusNotImplemented)
//		return
//	}

//	err = parser.ParseData(t.Content)
//	if err != nil {
//		httpError(w, err, http.StatusInternalServerError)
//		return
//	}

//	w.WriteHeader(http.StatusOK)
//}

func postTemplate(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	req := structs.ConfigTemplate{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	parser, err := factory(req.Image)
	if err != nil {
		httpError(w, err, http.StatusNotImplemented)
		return
	}

	parser = parser.clone(&req)

	err = parser.ParseData([]byte(req.Content))
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	dat, err := json.Marshal(req)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	path := make([]string, 1, 3)
	path[0] = imageKey
	path = append(path, strings.SplitN(req.Image, ":", 2)...)

	key := strings.Join(path, "/")
	err = ctx.client.PutKV(key, dat)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func generateConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	req := structs.ServiceSpec{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	parser, err := factory(req.Service.Image)
	if err != nil {
		httpError(w, err, http.StatusNotImplemented)
		return
	}

	var image, version string
	parts := strings.SplitN(req.Service.Image, ":", 2)
	if len(parts) == 2 {
		image, version = parts[0], parts[1]
	} else {
		image = parts[0]
	}

	key := strings.Join([]string{imageKey, image, version}, "/")
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

		parser = parser.clone(&t)

		err = parser.ParseData([]byte(t.Content))
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		err = parser.GenerateConfig(req.Units[i].ID, req)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		text, err := parser.Marshal()
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
			Name:         image,
			Version:      version,
			LogMount:     t.LogMount,
			DataMount:    t.DataMount,
			Content:      string(text),
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

	var pairs api.KVPairs

	switch len(req) {
	case 0:
		httpError(w, errors.New("no data need update"), http.StatusBadRequest)
		return
	case 1:
		for _, c := range req {
			key := strings.Join([]string{configKey, service, c.ID}, "/")
			pair, err := ctx.client.GetKV(key)
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}
			pairs = api.KVPairs{pair}
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

		if u.LogMount != "" && c.LogMount != u.LogMount {
			c.LogMount = u.LogMount
		}
		if u.DataMount != "" && c.DataMount != u.DataMount {
			c.DataMount = u.DataMount
		}

		if u.Content != "" && c.Content != u.Content {
			parser, err := factory(c.Name + ":" + c.Version)
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}

			parser = parser.clone(nil)

			err = parser.ParseData([]byte(u.Content))
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}

			c.Content = u.Content
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

func composeService(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	var req structs.ServiceSpec

	ip := ctx.mgmIP
	port := ctx.mgmPort

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	composer, err := compose.NewCompserBySpec(&req, ip, port)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	if err := composer.ComposeCluster(); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

}

func linkServices(ctx *_Context, w http.ResponseWriter, r *http.Request) {
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

package parser

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/plugin/parser/compose"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type _Context struct {
	apiVersion string
	client     kvstore.Store
	context    context.Context

	scriptDir string

	mgmIP   string
	mgmPort int
}

// NewRouter returns a pointer of mux.Router,router of plugin HTTP APIs.
func NewRouter(c kvstore.Client, kvpath, dir, ip string, port int) *mux.Router {
	type handler func(ctx *_Context, w http.ResponseWriter, r *http.Request)

	setLeaderElectionPath(kvpath)

	ctx := &_Context{
		client:    c,
		mgmIP:     ip,
		mgmPort:   port,
		scriptDir: dir,
	}

	var routes = map[string]map[string]handler{
		"GET": {
			"/image/template/{name}":       getImage,
			"/image/support":               getSupportImageVersion,
			"/configs/{service}":           getConfigs,
			"/configs/{service}/{unit:.*}": getConfig,
			"/commands/{service:.*}":       getCommands,
		},
		"POST": {
			"/configs":           generateConfigs,
			"/configs/{unit:.*}": generateConfig,
			"/image/template":    postTemplate,
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

func getImage(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	path := make([]string, 1, 3)
	path[0] = imageKey
	path = append(path, strings.SplitN(name, ":", 2)...)
	key := strings.Join(path, "/")

	pair, err := ctx.client.GetKV(ctx.context, key)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(pair.Value)
}

func getSupportImageVersion(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	out := make([]structs.ImageVersion, 0, 10)

	for key := range images {
		out = append(out, key)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}

func getConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	service := mux.Vars(r)["service"]

	cm, err := getConfigMapFromStore(ctx.context, ctx.client, service)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	image, version := "", ""
	for _, v := range cm {
		image = v.Name
		version = v.Version
		break
	}

	t, err := getTemplateFromStore(ctx.context, ctx.client, image, version)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	out, err := getServiceConfigResponse(service, cm, t)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}

func getServiceConfigResponse(service string, cm structs.ConfigsMap, t structs.ConfigTemplate) ([]structs.UnitConfig, error) {
	out := make([]structs.UnitConfig, 0, len(cm))
	var (
		pr  parser
		err error
	)

	for _, cc := range cm {
		image := cc.Name + ":" + cc.Version

		uc := structs.UnitConfig{
			ID:      cc.ID,
			Service: service,
			Cmds:    cc.Cmds,
			ConfigTemplate: structs.ConfigTemplate{
				Image:      image,
				LogMount:   cc.LogMount,
				DataMount:  cc.DataMount,
				ConfigFile: cc.ConfigFile,
				Content:    cc.Content,
				Keysets:    t.Keysets,
				Timestamp:  cc.Timestamp,
			},
		}

		if pr == nil {
			pr, err = factory(image)
			if err != nil {
				return out, err
			}
		}
		pr = pr.clone(&t)

		err = pr.ParseData([]byte(cc.Content))
		if err != nil {
			return out, err
		}

		for i := range uc.Keysets {
			uc.Keysets[i].Value = pr.get(uc.Keysets[i].Key)
		}

		out = append(out, uc)
	}

	return out, nil
}

func getConfig(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	service := mux.Vars(r)["service"]
	unit := mux.Vars(r)["unit"]
	key := strings.Join([]string{configKey, service, unit}, "/")

	// structs.ConfigCmds,encode by JSON
	pair, err := ctx.client.GetKV(ctx.context, key)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(pair.Value)
}

func getCommands(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	service := mux.Vars(r)["service"]

	cm, err := getConfigMapFromStore(ctx.context, ctx.client, service)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	resp := make(structs.Commands, len(cm))

	for _, c := range cm {
		resp[c.ID] = c.Cmds
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

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

	if len(req.Content) > 0 {
		err = parser.ParseData([]byte(req.Content))
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		for i := range req.Keysets {
			req.Keysets[i].Value = parser.get(req.Keysets[i].Key)
		}
	} else {
		for i, ks := range req.Keysets {
			err = parser.set(ks.Key, ks.Default)
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}

			req.Keysets[i].Value = req.Keysets[i].Default
		}

		out, err := parser.Marshal()
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		req.Content = string(out)
	}

	err = putTemplateToStore(ctx.context, ctx.client, req)
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

	logrus.Debugf("%+v", req)

	out, err := generateServiceConfigs(ctx.context, ctx.client, req, "")
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(out)
}

func generateConfig(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["unit"]
	req := structs.ServiceSpec{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	out, err := generateServiceConfigs(ctx.context, ctx.client, req, name)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	var cc *structs.ConfigCmds

	if val, ok := out[name]; ok {
		cc = &val
	} else if len(out) == 1 {
		for _, val := range out {
			cc = &val
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cc)

}

func updateConfigs(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	var (
		req     structs.ServiceConfigs
		service = mux.Vars(r)["service"]
	)

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}

	if len(req) == 0 {
		httpError(w, errors.New("no data need update"), http.StatusBadRequest)
		return
	}

	configs, err := getConfigMapFromStore(ctx.context, ctx.client, service)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	var (
		pr  parser
		out = make(structs.ConfigsMap, len(configs))
	)
	for _, u := range req {
		if pr == nil {
			pr, err = factory(u.Image)
			if err != nil {
				httpError(w, err, http.StatusInternalServerError)
				return
			}
		}

		cc, err := mergeUnitConfig(pr, u, configs[u.ID])
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}

		out[cc.ID] = cc

		err = putConfigsToStore(ctx.context, ctx.client, service, map[string]structs.ConfigCmds{
			cc.ID: cc,
		})
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}

func mergeUnitConfig(pr parser, uc structs.UnitConfig, cc structs.ConfigCmds) (structs.ConfigCmds, error) {
	if uc.ID != "" && cc.ID != uc.ID {
		cc.ID = uc.ID
	}

	if uc.LogMount != "" && cc.LogMount != uc.LogMount {
		cc.LogMount = uc.LogMount
	}
	if uc.DataMount != "" && cc.DataMount != uc.DataMount {
		cc.DataMount = uc.DataMount
	}
	if uc.ConfigFile != "" && cc.ConfigFile != uc.ConfigFile {
		cc.ConfigFile = uc.ConfigFile
	}

	pr = pr.clone(nil)
	err := pr.ParseData([]byte(cc.Content))
	if err != nil {
		return cc, err
	}

	for _, ks := range uc.Keysets {
		err := pr.set(ks.Key, ks.Value)
		if err != nil {
			return cc, err
		}
	}

	content, err := pr.Marshal()
	if err != nil {
		return cc, err
	}

	cc.Content = string(content)
	cc.Timestamp = time.Now().Unix()

	return cc, nil
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

	composer, err := compose.NewCompserBySpec(&req, ctx.scriptDir, ip, port)
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

	w.WriteHeader(status)
}

func generateServiceConfigs(ctx context.Context, kvc kvstore.Store, req structs.ServiceSpec, unitID string) (structs.ConfigsMap, error) {
	if unitID != "" {
		found := false
		for i := range req.Units {
			if unitID == req.Units[i].ID || unitID == req.Units[i].Name {
				unitID = req.Units[i].ID
				found = true
				break
			}
		}

		if !found {
			return nil, errors.New("unknown unit by:" + unitID)
		}
	}

	parser, err := factory(req.Service.Image)
	if err != nil {
		return nil, err
	}

	var image, version string
	parts := strings.SplitN(req.Service.Image, ":", 2)
	if len(parts) == 2 {
		image, version = parts[0], parts[1]
	} else {
		image = parts[0]
	}

	template, err := getTemplateFromStore(ctx, kvc, image, version)
	if err != nil {
		return nil, err
	}

	cm, err := getConfigMapFromStore(ctx, kvc, req.Service.ID)
	if err != nil {
		// ignore error
	}

	resp := make(structs.ConfigsMap, len(req.Units))

	for i := range req.Units {
		if unitID != "" && req.Units[i].ID != unitID {
			continue
		}

		t := template

		if cc, ok := cm[req.Units[i].ID]; ok {
			t.ConfigFile = cc.ConfigFile
			t.Content = cc.Content
			t.DataMount = cc.DataMount
			t.LogMount = cc.LogMount
		}

		cc, err := generateUnitConfig(req.Units[i].ID, parser, t, req)
		if err != nil {
			return nil, err
		}

		cc.Name = image
		cc.Version = version

		resp[req.Units[i].ID] = cc
	}

	err = putConfigsToStore(ctx, kvc, req.ID, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func generateUnitConfig(unitID string, pr parser, t structs.ConfigTemplate, spec structs.ServiceSpec) (structs.ConfigCmds, error) {
	pr = pr.clone(&t)

	err := pr.ParseData([]byte(t.Content))
	if err != nil {
		return structs.ConfigCmds{}, err
	}

	err = pr.GenerateConfig(unitID, spec)
	if err != nil {
		return structs.ConfigCmds{}, err
	}

	text, err := pr.Marshal()
	if err != nil {
		return structs.ConfigCmds{}, err
	}

	cmds, err := pr.GenerateCommands(unitID, spec)
	if err != nil {
		return structs.ConfigCmds{}, err
	}

	r, err := pr.HealthCheck(unitID, spec)
	if err != nil {
		return structs.ConfigCmds{}, err
	}

	return structs.ConfigCmds{
		ID:           unitID,
		LogMount:     t.LogMount,
		DataMount:    t.DataMount,
		ConfigFile:   filepath.Join(t.DataMount, t.ConfigFile),
		Content:      string(text),
		Cmds:         cmds,
		Timestamp:    time.Now().Unix(),
		Registration: r,
	}, nil
}

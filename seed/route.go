package seed

import (
	"encoding/json"
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

var (
	scriptDir = "/usr/local/swarm-agent/scripts/seed/"
)

func init() {
	if dir := os.Getenv("SCRIPT_DIR"); dir != "" {
		scriptDir = dir
	}
}

type _Context struct {
	apiVersion string
	context    context.Context
}

//CommonRes common http requet response body msg
type CommonRes struct {
	Err string `json:"Err"`
}

func errCommonHanlde(w http.ResponseWriter, req *http.Request, err error) {
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	json.NewEncoder(w).Encode(CommonRes{Err: err.Error()})

	log.Errorf("%s %s,error:%s\n", req.Method+":"+req.URL.Path, err)
}

func writeJSON(w http.ResponseWriter, obj interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if obj != nil {
		err := json.NewEncoder(w).Encode(obj)
		if err != nil {
			log.Errorf("write JSON:%d,%s", status, err)
		}
	}
}

func getVersionHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("version:" + ctx.apiVersion + "\n"))
}

// NewRouter create API router.
func NewRouter(version string) *mux.Router {
	type handler func(ctx *_Context, w http.ResponseWriter, r *http.Request)

	ctx := &_Context{apiVersion: version}

	var routes = map[string]map[string]handler{
		"GET": {
			"/san/vglist": vgListHandle,
			"/version":    getVersionHandle,
		},
		"POST": {
			"/VolumeDriver.Update": updateHandle,
			"/volume/file/cp":      volumeFileCpHandle,

			"/san/vgcreate": vgCreateHandle,
			"/san/vgextend": vgExtendHandle,

			"/san/activate":   activateHandle,
			"/san/deactivate": deactivateHandle,

			"/san/vg/remove": removeVGHandle,

			"/network/create": networkCreateHandle,
		},
	}

	r := mux.NewRouter()
	for method, mappings := range routes {
		for route, fct := range mappings {
			log.WithFields(log.Fields{"method": method, "route": route}).Debug("Registering HTTP route")

			localRoute := route
			localFct := fct

			wrap := func(w http.ResponseWriter, r *http.Request) {
				log.WithFields(log.Fields{"method": r.Method, "uri": r.RequestURI}).Debug("HTTP request received")

				ctx.context = r.Context()

				localFct(ctx, w, r)
			}
			localMethod := method

			r.Path("/v{version:[0-9]+.[0-9]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			r.Path(localRoute).Methods(localMethod).HandlerFunc(wrap)
		}
	}

	return r
}

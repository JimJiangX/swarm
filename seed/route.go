package seed

import (
	"encoding/json"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

type _Context struct {
	apiVersion string
	context    context.Context
}

type CommonRes struct {
	Err string `json:"Err"`
}

func errCommonHanlde(w http.ResponseWriter, req *http.Request, err error) {
	bts, _ := json.Marshal(CommonRes{Err: err.Error()})
	// http.Error(w, string(bts), 400)
	w.Write(bts)
	log.Printf("the req %s exec error:%s\n", req.Method+":"+req.URL.Path, err.Error())
}

func GetVersionHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("version:" + ctx.apiVersion + "\n"))
	// log.Info("the req :", req.Method, req.URL.Path)
}

func NewRouter(version string) *mux.Router {
	type handler func(ctx *_Context, w http.ResponseWriter, r *http.Request)

	ctx := &_Context{apiVersion: version}

	var routes = map[string]map[string]handler{
		"GET": {
			"/san/vglist": VgList,
			"/version":    GetVersionHandle,
		},
		"POST": {

			"/ip/create":           IPCreate,
			"/VolumeDriver.Update": Update,
			"/volume/file/cp":      VolumeFileCp,

			"/san/vgcreate": VgCreate,
			"/san/vgextend": VgExtend,

			// "/san/vgblock":  VgBlock,
			"/san/activate":   Activate,
			"/san/deactivate": Deactivate,

			"/san/vg/remove": RemoveVG,
			"/ip/remove":     IpRemove,
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

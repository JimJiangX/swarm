package api

import (
	"encoding/json"
	stderr "errors"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/structs"
	goctx "golang.org/x/net/context"
)

var errUnsupportGarden = stderr.New("unsupport Garden yet")

// Emit an HTTP error and log it.
func httpError2(w http.ResponseWriter, err error, status int) {
	if err != nil {
		json.NewEncoder(w).Encode(structs.ResponseHead{
			Result:  false,
			Code:    status,
			Message: err.Error(),
		})
	}

	w.WriteHeader(status)

	logrus.WithField("status", status).Errorf("HTTP error: %+v", err)
}

func httpSucceed(w http.ResponseWriter, obj interface{}, status int) {
	resp := structs.CommandResponse{
		ResponseHead: structs.ResponseHead{
			Result: true,
			Code:   status,
		},
		Object: obj,
	}

	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		logrus.WithField("status", status).Errorf("JSON Encode error: %+v", err)
	}

	w.WriteHeader(status)
}

func postDC(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.RegisterDC{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError2(w, err, http.StatusBadRequest)
		return
	}
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil {
		httpError2(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err = gd.Register(req)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	httpSucceed(w, nil, http.StatusCreated)
}

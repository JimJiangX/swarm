package main

import (
	"github.com/docker/go-plugins-helpers/sdk"
)

const manifest = `{"Implements": ["ConfigurationDriver"]}`

func main() {
	h := sdk.NewHandler(manifest)

	h.ServeTCP()
	h.ServeUnix()
}

func initMux(h sdk.Handler) {
	h.HandleFunc("/", getConfigs)
}

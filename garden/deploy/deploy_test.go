package deploy

import (
	"testing"

	"github.com/docker/swarm/garden/structs"
)

func TestServicesLink(t *testing.T) {
	links := structs.ServicesLink{
		Mode: "modeXXXXXXX",
		Links: []*structs.ServiceLink{
			{
				ID:   "cccccccccc",
				Deps: []string{"dddddddddd", "eeeeeeeeee", "ffffffffff"},
			},
			{
				ID:   "aaaaaaaaaa",
				Deps: []string{"bbbbbbbbbb", "cccccccccc", "dddddddddd"},
			},
			{
				ID: "dddddddddd",
			},
			{
				ID: "bbbbbbbbbb",
			},
			{
				ID: "gggggggggg",
			},
		},
	}

	if ids := links.LinkIDs(); len(ids) != 7 {
		t.Error(ids)
	} else {
		t.Log(ids)
	}

	links.Sort()

	for i := range links.Links {
		t.Log(links.Links[i])
	}

}

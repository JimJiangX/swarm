package database

import (
	"encoding/json"
	"time"

	"github.com/docker/swarm/api/api/structs"
)

type Software struct {
	Enabled  bool   `db:"enabled"`
	ID       string `db:"id"`
	Name     string `db:"name"`
	ImageID  string `db:"image_id"`
	Version  string `db:"version"`
	Label    string `db:"label"`
	StoreURL string `db:"store_url"`

	Ports         string        `db:"ports"`
	ports         []PortBinding `db:"-"`
	configKeySets []string      `db:"-"`
	ConfigKeySets string        `db:"config_key_sets"`

	Template string                 `db:"template"`
	template map[string]interface{} `db:"-"`

	UploadAt time.Time `db:"upload_at"`
}

func (v Software) TableName() string {
	return "tb_software"
}

type PortBinding structs.PortBinding

func InsertNewSoftware(s structs.PostSoftware, imageID string) (Software, error) {
	v := Software{
		ID:       s.ID,
		Name:     s.Name,
		Version:  s.Version,
		ImageID:  imageID,
		Enabled:  s.Enabled,
		Label:    s.Label,
		StoreURL: s.StoreURL,
		ports:    make([]PortBinding, len(s.Ports)),
		UploadAt: time.Now(),
	}

	for i := range s.Ports {
		v.ports[i].Name = s.Ports[i].Name
		v.ports[i].Port = s.Ports[i].Port
	}
	data, err := json.Marshal(&v.ports)
	if err != nil {

	}
	v.Ports = string(data)
	// insert into DB
	return v, nil
}

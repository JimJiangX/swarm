package database

import (
	"encoding/json"
	"time"
)

type Software struct {
	Enabled  bool   `db:"enabled"`
	ID       string `db:"id"`
	Name     string `db:"name"`
	Version  string `db:"version"`
	ImageID  string `db:"image_id"`
	Labels   string `db:"labels"`
	StoreURL string `db:"store_url"`

	Ports         string `db:"ports"`           // []PortBinding
	ConfigKeySets string `db:"config_key_sets"` // map[string]interface{}

	Template string                 `db:"template"`
	template map[string]interface{} `db:"-"`

	UploadAt time.Time `db:"upload_at"`
}

func (v Software) TableName() string {
	return "tb_software"
}

type PortBinding struct {
	Name  string
	Proto string // tcp/udp
	Port  int
}

func (sw Software) UnmarshalPorts() ([]PortBinding, error) {
	if len(sw.Ports) == 0 {
		return []PortBinding{}, nil
	}

	var ports []PortBinding
	err := json.Unmarshal([]byte(sw.Ports), &ports)

	return ports, err
}

func (sw Software) UnmarshalConfigKeySets() (map[string]interface{}, error) {
	if len(sw.ConfigKeySets) == 0 {
		return map[string]interface{}{}, nil
	}

	var pairs map[string]interface{}
	err := json.Unmarshal([]byte(sw.ConfigKeySets), &pairs)

	return pairs, err
}

func (sw Software) InsertSoftware() error {

	return nil
}

func QueryImage(name, version string) (Software, error) {
	db, err := GetDB(true)
	if err != nil {
		return Software{}, err
	}

	sw := Software{}
	err = db.QueryRowx("SELECT * FROM tb_software WHERE name=? AND version=?", name, version).StructScan(&sw)

	return sw, err
}

func QueryImageByID(id string) (Software, error) {
	db, err := GetDB(true)
	if err != nil {
		return Software{}, err
	}

	sw := Software{}
	err = db.QueryRowx("SELECT * FROM tb_software WHERE id=? OR image_id=?", id, id).StructScan(&sw)

	return sw, err
}

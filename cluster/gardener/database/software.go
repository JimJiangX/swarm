package database

import "time"

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

type PortBinding struct {
	Name  string
	Proto string // tcp/udp
	Port  int
}

func (sw Software) InsertSoftware() error {

	return nil
}

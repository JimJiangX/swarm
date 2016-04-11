package database

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

type Image struct {
	Enabled bool   `db:"enabled"`
	ID      string `db:"id"`
	Name    string `db:"name"`
	Version string `db:"version"`
	ImageID string `db:"docker_image_id"`
	Labels  string `db:"label"`

	PortString string `db:"ports"` // []Port
	PortSlice  []Port `db:"-"`

	TemplateConfigID string    `db:"template_config_id"`
	UploadAt         time.Time `db:"upload_at"`
}

func (Image) TableName() string {
	return "tb_image"
}

func (image *Image) encode() error {
	if len(image.PortSlice) == 0 {
		return nil
	}

	data, err := json.Marshal(image.PortSlice)
	if err == nil {
		image.PortString = string(data)
	}

	return err
}

func (image *Image) decode() error {
	if len(image.PortString) == 0 {
		return nil
	}

	return json.Unmarshal([]byte(image.PortString), &image.PortSlice)
}

func TxInsertImage(image Image, config UnitConfig) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = image.encode()
	if err != nil {
		return err
	}

	query := "INSERT INTO tb_image (enabled,id,name,version,docker_image_id,label,ports,template_config_id,upload_at) VALUES (:enabled,:id,:name,:version,:docker_image_id,:label,:ports,:template_config_id,:upload_at)"

	_, err = tx.NamedExec(query, &image)
	if err != nil {
		return err
	}

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func QueryImage(name, version string) (Image, error) {
	db, err := GetDB(true)
	if err != nil {
		return Image{}, err
	}

	image := Image{}
	err = db.QueryRowx("SELECT * FROM tb_image WHERE name=? AND version=?", name, version).StructScan(&image)

	image.decode()

	return image, err
}

func QueryImageByID(id string) (Image, error) {
	db, err := GetDB(true)
	if err != nil {
		return Image{}, err
	}

	image := Image{}
	err = db.QueryRowx("SELECT * FROM tb_image WHERE id=? OR image_id=?", id, id).StructScan(&image)

	image.decode()

	return image, err
}

type UnitConfig struct {
	ID            string          `db:"id"`
	ImageID       string          `db:"image_id"`
	Path          string          `db:"config_file_path"`
	Version       int             `db:"version"`
	ParentID      string          `db:"parent_id"`
	Content       string          `db:"content"`         // map[string]interface{}
	ConfigKeySets string          `db:"config_key_sets"` // map[string]bool
	KeySets       map[string]bool `db:"-"`

	CreateAt time.Time `db:"create_at"`
}

func (u UnitConfig) TableName() string {
	return "tb_unit_config"
}

func (c *UnitConfig) encode() error {
	if len(c.KeySets) > 0 {
		data, err := json.Marshal(c.KeySets)
		if err != nil {
			return err
		}

		c.ConfigKeySets = string(data)
	}

	return nil
}

func (c *UnitConfig) decode() error {
	if len(c.ConfigKeySets) > 0 {
		err := json.Unmarshal([]byte(c.ConfigKeySets), &c.KeySets)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetUnitConfigByID(id string) (*UnitConfig, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	config := &UnitConfig{}
	query := "SELECT * FROM tb_unit_config WHERE id=? OR image_id=?"

	err = db.QueryRowx(query, id, id).StructScan(config)
	if err != nil {
		return nil, err
	}

	err = config.decode()

	return config, err
}

func TXInsertUnitConfig(tx *sqlx.Tx, config *UnitConfig) error {
	err := config.encode()
	if err != nil {
		return err
	}

	query := "INSERT INTO tb_unit_config (id,image_id,config_file_path,version,parent_id,content,create_at) VALUES (:id,:image_id,:config_file_path,:version,:parent_id,:content,:create_at)"

	_, err = tx.NamedExec(query, config)

	return err
}

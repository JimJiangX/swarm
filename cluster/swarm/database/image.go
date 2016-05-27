package database

import (
	"bytes"
	"encoding/json"
	"strings"
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
	Size    int    `db:"size"`

	TemplateConfigID string    `db:"template_config_id"`
	UploadAt         time.Time `db:"upload_at"`
}

func (Image) TableName() string {
	return "tb_image"
}

func TxInsertImage(image Image, config UnitConfig, task Task) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := "INSERT INTO tb_image (enabled,id,name,version,docker_image_id,label,size,template_config_id,upload_at) VALUES (:enabled,:id,:name,:version,:docker_image_id,:label,:size,:template_config_id,:upload_at)"

	_, err = tx.NamedExec(query, &image)
	if err != nil {
		return err
	}

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return err
	}

	err = TxInsertTask(tx, task)
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
	err = db.Get(&image, "SELECT * FROM tb_image WHERE name=? AND version=?", name, version)

	return image, err
}

func GetImageAndUnitConfig(ID string) (Image, UnitConfig, error) {
	db, err := GetDB(true)
	if err != nil {
		return Image{}, UnitConfig{}, err
	}

	image := Image{}
	err = db.Get(&image, "SELECT * FROM tb_image WHERE id=? OR docker_image_id=?", ID, ID)

	config := UnitConfig{}
	err = db.Get(&config, "SELECT * FROM tb_unit_config WHERE id=?", image.TemplateConfigID)
	if err != nil {
		return Image{}, UnitConfig{}, err
	}

	err = config.decode()

	return image, config, err
}

func QueryImageByID(ID string) (Image, error) {
	db, err := GetDB(true)
	if err != nil {
		return Image{}, err
	}

	image := Image{}
	err = db.Get(&image, "SELECT * FROM tb_image WHERE id=? OR docker_image_id=?", ID, ID)

	return image, err
}

func UpdateImageStatus(ID string, enable bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_image SET enabled=? WHERE id=? OR docker_image_id=?", enable, ID, ID)

	return err
}

func DeleteImage(ID string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_image WHERE id=? OR docker_image_id=?", ID, ID)

	return err
}

type UnitConfig struct {
	ID            string                  `db:"id"`
	ImageID       string                  `db:"image_id"`
	Mount         string                  `db:"config_file_path"`
	Version       int                     `db:"version"`
	ParentID      string                  `db:"parent_id"`
	Content       string                  `db:"content"`         // map[string]interface{}
	configKeySets string                  `db:"config_key_sets"` // map[string]KeysetParams
	KeySets       map[string]KeysetParams `db:"-"`

	CreatedAt time.Time `db:"created_at"`
}

type KeysetParams struct {
	Key         string
	CanSet      bool
	MustRestart bool
	Description string
}

func (u UnitConfig) TableName() string {
	return "tb_unit_config"
}

func (c *UnitConfig) encode() error {
	buffer := bytes.NewBuffer(nil)
	err := json.NewEncoder(buffer).Encode(c.KeySets)

	if err == nil {
		c.configKeySets = buffer.String()
	}

	return nil
}

func (c *UnitConfig) decode() error {
	var val map[string]KeysetParams
	dec := json.NewDecoder(strings.NewReader(c.configKeySets))
	err := dec.Decode(&val)
	if err == nil {
		c.KeySets = val
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
	err = db.Get(config, query, id, id)
	if err != nil {
		return nil, err
	}

	err = config.decode()
	if err != nil {
		return nil, err
	}

	return config, err
}

func TXInsertUnitConfig(tx *sqlx.Tx, config *UnitConfig) error {
	err := config.encode()
	if err != nil {
		return err
	}

	query := "INSERT INTO tb_unit_config (id,image_id,config_file_path,version,parent_id,content,config_key_sets,created_at) VALUES (:id,:image_id,:config_file_path,:version,:parent_id,:content,:config_key_sets,:created_at)"

	_, err = tx.NamedExec(query, config)

	return err
}

func UpdateUnitConfig(config UnitConfig) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	err = config.encode()
	if err != nil {
		return err
	}

	query := "UPDATE tb_unit_config SET image_id=:image,config_file_path=:config_file_path,version=:version,parent_id=:parent_id,content=:content,config_key_sets=:config_key_sets,created_at=:created_at WHERE id=:id"

	_, err = db.NamedExec(query, config)

	return err
}

func DeleteUnitConfig(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_unit_config WHERE id=?", id)

	return err
}

func txDeleteUnitConfig(tx *sqlx.Tx, id string) error {
	_, err := tx.Exec("DELETE FROM tb_unit_config WHERE id=?", id)

	return err
}

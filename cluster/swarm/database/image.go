package database

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	TemplateConfigID string `db:"template_config_id"`
	ConfigKeySets    string `db:"config_keysets"` // map[string]KeysetParams

	UploadAt time.Time `db:"upload_at"`
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

	image.ConfigKeySets, err = unitConfigEncode(config.KeySets)

	query := "INSERT INTO tb_image (enabled,id,name,version,docker_image_id,label,size,template_config_id,config_keysets,upload_at) VALUES (:enabled,:id,:name,:version,:docker_image_id,:label,:size,:template_config_id,:config_keysets,:upload_at)"

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
	image := Image{}
	config := UnitConfig{}

	db, err := GetDB(true)
	if err != nil {
		return image, config, err
	}

	err = db.Get(&image, "SELECT * FROM tb_image WHERE id=? OR docker_image_id=?", ID, ID)
	if err != nil {
		return image, config, err
	}

	err = db.Get(&config, "SELECT * FROM tb_unit_config WHERE id=?", image.TemplateConfigID)
	if err != nil {
		return image, config, err
	}

	config.KeySets, err = unitConfigDecode(image.ConfigKeySets)

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
	ID        string                  `db:"id"`
	ImageID   string                  `db:"image_id"`
	Mount     string                  `db:"config_file_path"`
	Version   int                     `db:"version"`
	ParentID  string                  `db:"parent_id"`
	Content   string                  `db:"content"`
	KeySets   map[string]KeysetParams `db:"-"`
	CreatedAt time.Time               `db:"created_at"`
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

func unitConfigEncode(keysets map[string]KeysetParams) (string, error) {
	if len(keysets) == 0 {
		return "", nil
	}
	buffer := bytes.NewBuffer(nil)
	err := json.NewEncoder(buffer).Encode(keysets)

	return buffer.String(), err
}

func unitConfigDecode(src string) (map[string]KeysetParams, error) {
	if len(src) == 0 {
		return map[string]KeysetParams{}, nil
	}
	var val map[string]KeysetParams
	dec := json.NewDecoder(strings.NewReader(src))
	err := dec.Decode(&val)

	return val, err
}

func GetUnitConfigByID(id string) (*UnitConfig, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	config := &UnitConfig{}
	query := "SELECT * FROM tb_unit_config WHERE id=?"
	err = db.Get(config, query, id)
	if err != nil {
		return nil, err
	}

	var keysets string
	err = db.Get(&keysets, "SELECT config_keysets FROM tb_image WHERE id=?", config.ImageID)
	if err != nil {
		return nil, err
	}

	config.KeySets, err = unitConfigDecode(keysets)

	return config, err
}

func ListUnitConfigByService(service string) ([]struct {
	Unit   Unit
	Config UnitConfig
}, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var units []Unit
	err = db.Select(&units, "SELECT * FROM tb_unit WHERE service_id=?", service)
	if err != nil {
		return nil, err
	}

	if len(units) == 0 {
		return nil, fmt.Errorf("Not Found Units of Service,Is '%s' Exist?", service)
	}

	ids := make([]string, len(units))
	for i := range units {
		ids[i] = units[i].ConfigID
	}

	query, args, err := sqlx.In("SELECT * FROM tb_unit_config WHERE id IN (?);", ids)
	if err != nil {
		return nil, err
	}

	var configs []*UnitConfig
	err = db.Select(&configs, query, args...)
	if err != nil {
		return nil, err
	}

	if len(configs) == 0 {
		return nil, nil
	}

	var result []struct {
		ID      string `db:"id"`
		KeySets string `db:"config_keysets"`
	}
	err = db.Select(&result, "SELECT id,config_keysets FROM tb_image")
	if err != nil {
		return nil, err
	}

	for i := range configs {
		for r := range result {
			if configs[i].ImageID != result[r].ID {
				continue
			}
			configs[i].KeySets, err = unitConfigDecode(result[r].KeySets)
			if err == nil {
				break
			}
		}
	}

	out := make([]struct {
		Unit   Unit
		Config UnitConfig
	}, 0, len(units))

	for i := range units {
		for _, c := range configs {
			if c == nil {
				break
			}

			if units[i].ConfigID != c.ID {
				continue
			}
			out = append(out, struct {
				Unit   Unit
				Config UnitConfig
			}{
				Unit:   units[i],
				Config: *c,
			})
			break
		}
	}

	return out, nil
}

func TXInsertUnitConfig(tx *sqlx.Tx, config *UnitConfig) error {
	query := "INSERT INTO tb_unit_config (id,image_id,config_file_path,version,parent_id,content,created_at) VALUES (:id,:image_id,:config_file_path,:version,:parent_id,:content,:created_at)"
	_, err := tx.NamedExec(query, config)

	return err
}

func TxUpdateImageTemplateConfig(image string, config UnitConfig) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return err
	}

	if keysets := ""; len(config.KeySets) == 0 {
		_, err = tx.Exec("UPDATE tb_image SET template_config_id=? WHERE id=?", config.ID, image)
	} else {
		keysets, err = unitConfigEncode(config.KeySets)
		if err != nil {
			return err
		}

		_, err = tx.Exec("UPDATE tb_image SET template_config_id=?,config_keysets=? WHERE id=?", config.ID, keysets, image)
	}

	if err != nil {
		return err
	}

	return tx.Commit()
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

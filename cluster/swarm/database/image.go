package database

import (
	"encoding/json"
	"time"
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

func InsertImage(image Image) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	image.encode()

	query := "INSERT INTO tb_image (enabled,id,name,version,docker_image_id,label,ports,template_config_id,upload_at) VALUES (:enabled,:id,:name,:version,:docker_image_id,:label,:ports,:template_config_id,:upload_at)"

	_, err = db.NamedExec(query, image)

	return err
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

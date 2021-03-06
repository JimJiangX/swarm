package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/docker/swarm/garden/structs"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type ImageOrmer interface {
	SysConfigOrmer
	ImageIface
	TaskOrmer
}

type ImageIface interface {
	GetImageVersion(name string) (Image, error)
	//	GetImage(name string, major, minor, patch, build int) (Image, error)

	ListImages() ([]Image, error)

	InsertImageWithTask(img Image, t Task) error
	SetImageAndTask(img Image, t Task) error

	DelImage(ID string) error
}

// Image table structure,correspod with docker image.
type Image struct {
	ID       string    `db:"id"`
	Name     string    `db:"software_name"`
	ImageID  string    `db:"docker_image_id"`
	Major    int       `db:"major_version"`
	Minor    int       `db:"minor_version"`
	Patch    int       `db:"patch_version"`
	Dev      int       `db:"build_version"`
	Size     int       `db:"size"`
	Labels   string    `db:"label"`
	UploadAt time.Time `db:"upload_at"`
}

func (db dbBase) imageTable() string {
	return db.prefix + "_software_image"
}

func (im Image) Image() string {
	return fmt.Sprintf("%s:%d.%d.%d.%d", im.Name, im.Major, im.Minor, im.Patch, im.Dev)
}

// InsertImage insert Image
func (db dbBase) InsertImageWithTask(img Image, t Task) error {
	do := func(tx *sqlx.Tx) error {
		query := "INSERT INTO " + db.imageTable() + " (id,software_name,docker_image_id,major_version,minor_version,patch_version,build_version,size,label,upload_at) VALUES (:id,:software_name,:docker_image_id,:major_version,:minor_version,:patch_version,:build_version,:size,:label,:upload_at)"

		_, err := tx.NamedExec(query, &img)
		if err != nil {
			return errors.Wrap(err, "insert Image")
		}

		return db.txInsertTask(tx, t, db.imageTable())
	}

	return db.txFrame(do)
}

// ListImages returns Image slice select for DB.
func (db dbBase) ListImages() ([]Image, error) {
	var (
		images []Image
		query  = "SELECT id,software_name,docker_image_id,major_version,minor_version,patch_version,build_version,size,label,upload_at FROM " + db.imageTable()
	)

	err := db.Select(&images, query)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return images, errors.Wrap(err, "list []Image")
}

func (db dbBase) GetImageVersion(nameOrID string) (Image, error) {
	image := Image{}
	query := "SELECT id,software_name,docker_image_id,major_version,minor_version,patch_version,build_version,size,label,upload_at FROM " + db.imageTable() + " WHERE id=? OR docker_image_id=?"
	err := db.Get(&image, query, nameOrID, nameOrID)
	if err == nil {
		return image, nil
	}

	im, err := structs.ParseImage(nameOrID)
	if err == nil {
		return db.GetImage(im.Name, im.Major, im.Minor, im.Patch, im.Dev)
	}

	return image, errors.Wrap(err, "get image by id:"+nameOrID)
}

// GetImage returns Image select by name and version.
func (db dbBase) GetImage(name string, major, minor, patch, build int) (Image, error) {
	image := Image{}
	query := "SELECT id,software_name,docker_image_id,major_version,minor_version,patch_version,build_version,size,label,upload_at FROM " + db.imageTable() + " WHERE software_name=? AND major_version=? AND minor_version=? AND patch_version=? AND build_version=?"

	err := db.Get(&image, query, name, major, minor, patch, build)

	return image, errors.Wrap(err, "get Image")
}

// SetImageStatus update Image.ImageID&Size by ID.
func (db dbBase) SetImageAndTask(img Image, t Task) error {
	do := func(tx *sqlx.Tx) error {
		query := "UPDATE " + db.imageTable() + " SET docker_image_id=?,size=?,upload_at=? WHERE id=?"

		_, err := db.Exec(query, img.ImageID, img.Size, img.UploadAt, img.ID)
		if err != nil {
			return errors.Wrap(err, "update Image by ID")
		}

		return db.txSetTask(tx, t)
	}

	return db.txFrame(do)
}

// DelImage delete Image by ID in Tx.
func (db dbBase) DelImage(ID string) error {
	do := func(tx *sqlx.Tx) error {

		n := 0
		query := "SELECT COUNT(id) FROM " + db.serviceDescTable() + " WHERE image_id=?"

		err := tx.Get(&n, query, ID)
		if err != nil {
			return errors.Wrap(err, "Count Service filter by id")
		}

		if n > 0 {
			return errors.Errorf("image:%s is used %d", ID, n)
		}

		_, err = tx.Exec("DELETE FROM "+db.imageTable()+" WHERE id=?", ID)

		return errors.Wrapf(err, "Tx delete Imgage by ID:%s", ID)
	}

	return db.txFrame(do)
}

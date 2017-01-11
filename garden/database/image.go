package database

import (
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type ImageOrmer interface {
	GetImage(nameOrID string) (Image, error)
	ListImages() ([]Image, error)

	InsertImage(image Image) error
	SetImageStatus(ID string, enable bool) error

	DelImage(ID string) error
}

// Image table structure,correspod with docker image.
type Image struct {
	Enabled  bool      `db:"enabled"`
	ID       string    `db:"id"`
	Name     string    `db:"name"`
	Version  string    `db:"version"`
	Labels   string    `db:"label"`
	Size     int       `db:"size"`
	UploadAt time.Time `db:"upload_at"`
}

func (db dbBase) imageTable() string {
	return db.prefix + "_image"
}

// InsertImage insert Image
func (db dbBase) InsertImage(image Image) error {
	query := "INSERT INTO " + db.imageTable() + " (enabled,id,name,version,label,size,upload_at) VALUES (:enabled,:id,:name,:version,:label,:size,:upload_at)"

	_, err := db.NamedExec(query, &image)

	return errors.Wrap(err, "insert Image")
}

// ListImages returns Image slice select for DB.
func (db dbBase) ListImages() ([]Image, error) {
	var (
		images []Image
		query  = "SELECT enabled,id,name,version,label,size,upload_at FROM " + db.imageTable()
	)

	err := db.Select(&images, query)

	return images, errors.Wrap(err, "list []Image")
}

// GetImage returns Image select by name and version.
func (db dbBase) GetImage(nameOrID string) (Image, error) {
	var (
		image Image
		err   error
	)

	parts := strings.Split(nameOrID, ":")
	if len(parts) == 2 {
		query := "SELECT enabled,id,name,version,label,size,upload_at FROM " + db.imageTable() + " WHERE name=? AND version=?"
		err = db.Get(&image, query, parts[0], parts[0])
	} else {
		query := "SELECT enabled,id,name,version,label,size,upload_at FROM " + db.imageTable() + " WHERE id=?"
		err = db.Get(&image, query, nameOrID)
	}

	return image, errors.Wrap(err, "get Image")
}

// SetImageStatus update Image.Enabled by ID or ImageID.
func (db dbBase) SetImageStatus(ID string, enable bool) error {

	query := "UPDATE " + db.imageTable() + " SET enabled=? WHERE id=?"
	_, err := db.Exec(query, enable, ID)

	return errors.Wrap(err, "update Image by ID")
}

// DelImage delete Image by ID in Tx.
func (db dbBase) DelImage(ID string) error {

	do := func(tx *sqlx.Tx) error {
		var (
			image = Image{}
			query = "SELECT enabled,id,name,version,label,size,upload_at FROM " + db.imageTable() + " WHERE id=?"
		)

		err := tx.Get(&image, query, ID)
		if err != nil {
			if errors.Cause(err) == sql.ErrNoRows {
				return nil
			}

			return err
		}

		//		ok, err := isImageUsed(tx, image.ID)
		//		if err != nil {
		//			return err
		//		}
		//		if ok {
		//			return errors.Errorf("Image %s is using", ID)
		//		}

		_, err = tx.Exec("DELETE FROM "+db.imageTable()+" WHERE id=?", ID)

		return errors.Wrapf(err, "Tx delete Imgage by ID:%s", image.ID)
	}

	return db.txFrame(do)
}

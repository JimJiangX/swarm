package database

import (
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type VolumeOrmer interface {
	GetVolume(nameOrID string) (Volume, error)
	ListVolumeByVG(vg string) ([]Volume, error)
	ListVolumesByUnitID(id string) ([]Volume, error)

	InsertVolume(lv Volume) error
	InsertVolumes(lvs []Volume) error

	SetVolume(nameOrID string, size int) error
	SetVolumes(lvs []Volume) error

	DelVolume(nameOrID string) error
	DelVolumes(lvs []Volume) error
}

// Volume is table structure,correspod with host LV
type Volume struct {
	Size       int64  `db:"size"`
	ID         string `db:"id"`
	Name       string `db:"name"`
	UnitID     string `db:"unit_id"`
	VGName     string `db:"VGname"`
	Driver     string `db:"driver"`
	Filesystem string `db:"fstype"`
}

func (db dbBase) volumeTable() string {
	return db.prefix + "_volumes"
}

// InsertVolume insert a new Volume
func (db dbBase) InsertVolume(lv Volume) error {

	query := "INSERT INTO " + db.volumeTable() + " (id,name,unit_id,size,VGname,driver,fstype) VALUES (:id,:name,:unit_id,:size,:VGname,:driver,:fstype)"

	_, err := db.NamedExec(query, &lv)

	return errors.Wrap(err, "insert Volume")
}

// InsertVolumes insert new Volumes
func (db dbBase) InsertVolumes(lvs []Volume) error {

	do := func(tx *sqlx.Tx) error {
		query := "INSERT INTO " + db.volumeTable() + " (id,name,unit_id,size,VGname,driver,fstype) VALUES (:id,:name,:unit_id,:size,:VGname,:driver,:fstype)"

		stmt, err := tx.PrepareNamed(query)
		if err != nil {
			return errors.Wrap(err, "tx prepare insert Volume")
		}

		for i := range lvs {
			_, err := stmt.Exec(&lvs[i])
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "tx insert Volume")
			}
		}

		stmt.Close()

		return nil
	}

	return db.txFrame(do)
}

// SetVolume update size of Volume by name or ID
func (db dbBase) SetVolume(nameOrID string, size int) error {

	query := "UPDATE " + db.volumeTable() + " SET size=? WHERE id=? OR name=?"

	_, err := db.Exec(query, size, nameOrID, nameOrID)

	return errors.Wrap(err, "update Volume size")
}

// SetVolumes update Size of Volume by name or ID in a Tx
func (db dbBase) SetVolumes(lvs []Volume) error {
	do := func(tx *sqlx.Tx) error {

		stmt, err := tx.Preparex("UPDATE " + db.volumeTable() + " SET size=? WHERE id=?")
		if err != nil {
			return errors.Wrap(err, "Tx prepare update local Volume")
		}

		for _, lv := range lvs {
			_, err := stmt.Exec(lv.Size, lv.ID)
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "Tx update Volume size")
			}
		}

		stmt.Close()

		return errors.Wrap(err, "Tx update Volume size")
	}

	return db.txFrame(do)
}

// txDelVolume delete Volume by name or ID
func (db dbBase) txDelVolume(tx *sqlx.Tx, nameOrID string) error {

	query := "DELETE FROM " + db.volumeTable() + " WHERE id=? OR name=?"

	_, err := tx.Exec(query, nameOrID, nameOrID)

	return errors.Wrap(err, "tx delete Volume by nameOrID")
}

// DelVolume delete Volume by name or ID
func (db dbBase) DelVolume(nameOrID string) error {

	query := "DELETE FROM " + db.volumeTable() + " WHERE id=? OR name=?"

	_, err := db.Exec(query, nameOrID, nameOrID)

	return errors.Wrap(err, "delete Volume by nameOrID")
}

// txDelVolumeByUnit delete Volume by name or ID
func (db dbBase) txDelVolumeByUnit(tx *sqlx.Tx, unitID string) error {

	query := "DELETE FROM " + db.volumeTable() + " WHERE unit_id=?"

	_, err := tx.Exec(query, unitID)

	return errors.Wrap(err, "delete Volume by unitID")
}

// DelVolumes delete []LocalVolume in a Tx.
func (db dbBase) DelVolumes(lvs []Volume) error {
	do := func(tx *sqlx.Tx) error {

		stmt, err := tx.Preparex("DELETE FROM " + db.volumeTable() + " WHERE id=?")
		if err != nil {
			return errors.Wrap(err, "Tx prepare delete []Volume")
		}

		for i := range lvs {
			_, err = stmt.Exec(lvs[i].ID)
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "Tx delete Volume:"+lvs[i].ID)
			}
		}

		stmt.Close()

		return errors.Wrap(err, "Tx delete []Volume")
	}

	return db.txFrame(do)
}

// GetVolume returns Volume select by name or ID
func (db dbBase) GetVolume(nameOrID string) (Volume, error) {
	lv := Volume{}

	query := "SELECT id,name,unit_id,size,VGname,driver,fstype FROM " + db.volumeTable() + " WHERE id=? OR name=?"

	err := db.Get(&lv, query, nameOrID, nameOrID)

	return lv, errors.Wrap(err, "get Volume by nameOrID")
}

// ListVolumeByVG returns []Volume select by VGName
func (db dbBase) ListVolumeByVG(name string) ([]Volume, error) {
	var (
		lvs   []Volume
		query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM " + db.volumeTable() + " WHERE VGname=?"
	)

	err := db.Select(&lvs, query, name)

	return lvs, errors.Wrap(err, "list []Volume by VGName")
}

// ListVolumesByUnitID returns []Volume select by UnitID
func (db dbBase) ListVolumesByUnitID(id string) ([]Volume, error) {
	var (
		lvs   []Volume
		query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM " + db.volumeTable() + " WHERE unit_id=?"
	)

	err := db.Select(&lvs, query, id)

	return lvs, errors.Wrap(err, "list []Volume by UnitID")
}

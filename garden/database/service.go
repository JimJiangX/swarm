package database

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type ServiceOrmer interface {
	UnitOrmer

	InsertService(svc Service, t *Task, users []User) error

	GetService(nameOrID string) (Service, error)
	GetServiceStatus(nameOrID string) (int, error)
	GetServiceByUnit(nameOrID string) (Service, error)

	ListService() ([]Service, error)

	UpdateServiceStatus(nameOrID string, val int, finish time.Time) error
	UpdateServcieDesc(id, desc string, size int) error
	ServiceStatusCAS(nameOrID string, val int, finish time.Time, f func(val int) bool) (bool, int, error)
	SetServiceStatus(svc *Service, state int, finish time.Time) error
	UpdateServiceWithTask(svc *Service, t Task, state int, finish time.Time) error

	DeteleServiceRelation(serviceID string, rmVolumes bool) error
}

// Service if table structure
type Service struct {
	ID                string `db:"id"`
	Name              string `db:"name"`
	Desc              string `db:"description"` // short for Description
	Architecture      string `db:"architecture"`
	BusinessCode      string `db:"business_code"`
	AutoHealing       bool   `db:"auto_healing"`
	AutoScaling       bool   `db:"auto_scaling"`
	Status            int    `db:"status"`
	BackupMaxSizeByte int    `db:"backup_max_size"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	BackupFilesRetention int       `db:"backup_files_retention"`
	CreatedAt            time.Time `db:"created_at"`
	FinishedAt           time.Time `db:"finished_at"`
}

func (db dbBase) serviceTable() string {
	return db.prefix + "_service"
}

// ListService returns all []Service
func (db dbBase) ListService() ([]Service, error) {
	var (
		out   []Service
		query = "SELECT id,name,description,architecture,business_code,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable() + ""
	)

	err := db.Select(&out, query)

	return out, errors.Wrap(err, "list []Service")
}

// GetService returns Service select by ID or Name
func (db dbBase) GetService(nameOrID string) (Service, error) {
	var (
		s     = Service{}
		query = "SELECT id,name,description,architecture,business_code,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	)

	err := db.Get(&s, query, nameOrID, nameOrID)

	return s, errors.Wrap(err, "get Service by nameOrID")
}

// GetServiceStatus returns Service Status select by ID or Name
func (db dbBase) GetServiceStatus(nameOrID string) (int, error) {
	var (
		n     int
		query = "SELECT status FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	)

	err := db.Get(&n, query, nameOrID, nameOrID)

	return n, errors.Wrap(err, "get Service.Status by nameOrID")
}

// GetServiceByUnit returns Service select by Unit ID or Name.
func (db dbBase) GetServiceByUnit(nameOrID string) (Service, error) {

	var (
		queryUnit    = "SELECT service_id FROM " + db.unitTable() + " WHERE id=? OR name=?"
		queryService = "SELECT id,name,description,architecture,business_code,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable() + " WHERE id=?"

		id      string
		service Service
	)

	err := db.Get(&id, queryUnit, nameOrID, nameOrID)
	if err != nil {
		return service, errors.Wrap(err, "get Unit")
	}

	err = db.Get(&service, queryService, id)
	if err != nil {
		return service, errors.Wrap(err, "get Service")
	}

	return service, errors.Wrap(err, "get Service by unit")
}

// UpdateServcieDesc update Service Description
func (db dbBase) UpdateServiceStatus(nameOrID string, val int, finish time.Time) error {

	query := "UPDATE " + db.serviceTable() + " SET status=?,finished_at=? WHERE id=? OR name=?"

	_, err := db.Exec(query, val, finish, nameOrID, nameOrID)

	return errors.Wrap(err, "update Service Status")
}

// UpdateServcieDesc update Service Description
func (db dbBase) UpdateServcieDesc(id, desc string, size int) error {

	query := "UPDATE " + db.serviceTable() + " SET backup_max_size=?,description=? WHERE id=?"

	_, err := db.Exec(query, size, desc, id)

	return errors.Wrap(err, "update Service.Desc")
}

func (db dbBase) ServiceStatusCAS(nameOrID string, val int, finish time.Time, f func(val int) bool) (bool, int, error) {
	var (
		status int
		done   bool
	)

	do := func(tx *sqlx.Tx) error {

		query := "SELECT status FROM " + db.serviceTable() + " WHERE id=? OR name=?"

		err := tx.Get(&status, query, nameOrID, nameOrID)
		if err != nil {
			return errors.Wrap(err, "Tx get Service Status")
		}

		if !f(status) {
			return nil
		}

		query = "UPDATE " + db.serviceTable() + " SET status=?,finished_at=? WHERE id=? OR name=?"

		_, err = tx.Exec(query, val, finish, nameOrID, nameOrID)
		if err != nil {
			return errors.Wrap(err, "Tx update Service status")
		}

		done = true
		status = val

		return nil
	}

	err := db.txFrame(do)

	return done, status, err
}

// TxSaveService insert Service & Task & []User in Tx.
func (db dbBase) InsertService(svc Service, t *Task, users []User) error {
	do := func(tx *sqlx.Tx) error {

		err := db.txInsertSerivce(tx, svc)
		if err != nil {
			return err
		}

		if t != nil {
			err = db.txInsertTask(tx, *t)
			if err != nil {
				return err
			}
		}

		if len(users) > 0 {
			err = db.txInsertUsers(tx, users)
		}

		return err
	}

	return db.txFrame(do)
}

func (db dbBase) txInsertSerivce(tx *sqlx.Tx, svc Service) error {

	query := "INSERT INTO " + db.serviceTable() + " (id,name,description,architecture,business_code,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at) VALUES (:id,:name,:description,:architecture,:business_code,:auto_healing,:auto_scaling,:status,:backup_max_size,:backup_files_retention,:created_at,:finished_at)"

	_, err := tx.NamedExec(query, &svc)

	return errors.Wrap(err, "Tx insert Service")
}

// SetServiceStatus update Service Status
func (db dbBase) SetServiceStatus(svc *Service, state int, finish time.Time) error {
	if finish.IsZero() {

		query := "UPDATE " + db.serviceTable() + " SET status=? WHERE id=?"
		_, err := db.Exec(query, state, svc.ID)
		if err != nil {
			return errors.Wrap(err, "update Service Status")
		}

		svc.Status = state

		return nil
	}

	query := "UPDATE " + db.serviceTable() + " SET status=?,finished_at=? WHERE id=?"
	_, err := db.Exec(query, state, finish, svc.ID)
	if err != nil {
		return errors.Wrap(err, "update Service Status & FinishedAt")
	}

	svc.Status = state
	svc.FinishedAt = finish

	return nil
}

// TxSetServiceStatus update Service Status and Task Status in Tx.
func (db dbBase) UpdateServiceWithTask(svc *Service, t Task, state int, finish time.Time) error {
	do := func(tx *sqlx.Tx) (err error) {

		if finish.IsZero() {

			query := "UPDATE " + db.serviceTable() + " SET status=? WHERE id=?"
			_, err = tx.Exec(query, state, svc.ID)

		} else {

			query := "UPDATE " + db.serviceTable() + " SET status=?,finished_at=? WHERE id=?"
			_, err = tx.Exec(query, state, finish, svc.ID)

		}
		if err != nil {
			return errors.Wrap(err, "Tx update Service status")
		}

		err = db.txUpdateTask(tx, t)

		return err
	}

	err := db.txFrame(do)
	if err == nil {
		if !finish.IsZero() {
			svc.FinishedAt = finish
		}
		svc.Status = state
	}

	return err
}

func (db dbBase) txDeleteService(tx *sqlx.Tx, nameOrID string) error {

	query := "DELETE FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	_, err := tx.Exec(query, nameOrID, nameOrID)

	return err
}

// DeteleServiceRelation delelte related record about Service,
// include Service,Unit,BackupStrategy,IP,Port,LocalVolume,UnitConfig.
// delete in a Tx
func (db dbBase) DeteleServiceRelation(serviceID string, rmVolumes bool) error {
	units, err := db.ListUnitByServiceID(serviceID)
	if err != nil {
		return err
	}

	// recycle networking & ports & volumes
	ips := make([]IP, 0, 20)
	ports := make([]Port, 0, 20)
	volumes := make([]Volume, 0, 20)

	for i := range units {

		ipl, err := db.ListIPByUnitID(units[i].ID)
		if err == nil {
			ips = append(ips, ipl...)
		}

		pl, err := db.ListPortsByUnit(units[i].ID)
		if err == nil {
			ports = append(ports, pl...)
		}

		vl, err := db.ListVolumesByUnitID(units[i].ID)
		if err == nil {
			volumes = append(volumes, vl...)
		}

	}

	for i := range ips {
		ips[i].Allocated = false
		ips[i].UnitID = ""
	}

	for i := range ports {
		ports[i].Allocated = false
		ports[i].Name = ""
		ports[i].UnitID = ""
		ports[i].UnitName = ""
		ports[i].Proto = ""
	}

	do := func(tx *sqlx.Tx) error {

		err := db.UpdateIPs(tx, ips)
		if err != nil {
			return err
		}

		err = db.txUpdatePorts(tx, ports)
		if err != nil {
			return err
		}

		for i := range units {
			if rmVolumes {
				err = db.DeleteVolumeByUnit(tx, units[i].ID)
				if err != nil {
					return err
				}
			}

			//			err = db.txDeleteUnitConfigByUnit(tx, units[i].ID)
			//			if err != nil {
			//				return err
			//			}
		}

		//		err = db.txDeleteBackupStrategy(tx, serviceID)
		//		if err != nil {
		//			return err
		//		}

		err = db.txDeleteUsers(tx, serviceID)
		if err != nil {
			return err
		}

		err = db.DeleteUnit(tx, serviceID)
		if err != nil {
			return err
		}

		err = db.txDeleteService(tx, serviceID)

		return err
	}

	return db.txFrame(do)
}

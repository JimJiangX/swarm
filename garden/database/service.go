package database

import (
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type ServiceInterface interface {
	InsertService(svc Service, units []Unit, t *Task, users []User) error

	GetService(nameOrID string) (Service, error)
	GetServiceStatus(nameOrID string) (int, error)
	GetServiceByUnit(nameOrID string) (Service, error)

	ListServices() ([]Service, error)

	SetServiceStatus(nameOrID string, val int, finish time.Time) error
	SetServcieDesc(id, desc string, size int) error
	ServiceStatusCAS(nameOrID string, val int, finish time.Time, f func(val int) bool) (bool, int, error)
	SetServiceWithTask(svc *Service, t Task, state int, finish time.Time) error
}

type ServiceInfoInterface interface {
	GetServiceInfo(nameOrID string) (ServiceInfo, error)
	ListServicesInfo() ([]ServiceInfo, error)

	DelServiceRelation(serviceID string, rmVolumes bool) error
}

type ServiceOrmer interface {
	UnitOrmer

	UserOrmer

	ImageOrmer

	ServiceInterface

	ServiceInfoInterface
}

// Service if table structure
type Service struct {
	ID                string `db:"id"`
	Name              string `db:"name"`
	Image             string `db:"image"`       // imageName:imageVersion
	Desc              string `db:"description"` // short for Description
	Architecture      string `db:"architecture"`
	BusinessCode      string `db:"business_code"`
	Tag               string `db:"tag"`
	AutoHealing       bool   `db:"auto_healing"`
	AutoScaling       bool   `db:"auto_scaling"`
	Status            int    `db:"status"`
	BackupMaxSizeByte int    `db:"backup_max_size"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	BackupFilesRetention int       `db:"backup_files_retention"`
	CreatedAt            time.Time `db:"created_at"`
	FinishedAt           time.Time `db:"finished_at"`
}

func (s Service) ParseImage() (string, string) {
	parts := strings.SplitN(s.Image, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return s.Image, ""
}

func (db dbBase) serviceTable() string {
	return db.prefix + "_service"
}

// ListServices returns all []Service
func (db dbBase) ListServices() ([]Service, error) {
	var (
		out   []Service
		query = "SELECT id,name,description,architecture,business_code,tag,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable() + ""
	)

	err := db.Select(&out, query)

	return out, errors.Wrap(err, "list []Service")
}

// GetService returns Service select by ID or Name
func (db dbBase) GetService(nameOrID string) (Service, error) {
	var (
		s     = Service{}
		query = "SELECT id,name,description,architecture,business_code,tag,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable() + " WHERE id=? OR name=?"
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
		queryService = "SELECT id,name,description,architecture,business_code,tag,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable() + " WHERE id=?"

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

// SetServcieDesc update Service Description
func (db dbBase) SetServcieDesc(id, desc string, size int) error {

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
func (db dbBase) InsertService(svc Service, units []Unit, t *Task, users []User) error {
	do := func(tx *sqlx.Tx) error {

		err := db.txInsertSerivce(tx, svc)
		if err != nil {
			return err
		}

		if len(units) > 0 {
			err = db.txInsertUnits(tx, units)
			if err != nil {
				return err
			}
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

	query := "INSERT INTO " + db.serviceTable() + " (id,name,description,architecture,business_code,tag,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at) VALUES (:id,:name,:description,:architecture,:business_code,:tag,:auto_healing,:auto_scaling,:status,:backup_max_size,:backup_files_retention,:created_at,:finished_at)"

	_, err := tx.NamedExec(query, &svc)

	return errors.Wrap(err, "Tx insert Service")
}

// SetServiceStatus update Service Status
func (db dbBase) SetServiceStatus(nameOrID string, state int, finish time.Time) error {
	if finish.IsZero() {

		query := "UPDATE " + db.serviceTable() + " SET status=? WHERE id=? OR name=?"
		_, err := db.Exec(query, state, nameOrID, nameOrID)

		return errors.Wrap(err, "update Service Status")
	}

	query := "UPDATE " + db.serviceTable() + " SET status=?,finished_at=? WHERE id=? OR name=?"
	_, err := db.Exec(query, state, finish, nameOrID, nameOrID)

	return errors.Wrap(err, "update Service Status & FinishedAt")
}

// SetServiceWithTask update Service Status and Task Status in Tx.
func (db dbBase) SetServiceWithTask(svc *Service, t Task, state int, finish time.Time) error {
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

		err = db.txSetTask(tx, t)

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

func (db dbBase) txDelService(tx *sqlx.Tx, nameOrID string) error {

	query := "DELETE FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	_, err := tx.Exec(query, nameOrID, nameOrID)

	return err
}

// DelServiceRelation delelte related record about Service,
// include Service,Unit,BackupStrategy,IP,Port,LocalVolume,UnitConfig.
// delete in a Tx
func (db dbBase) DelServiceRelation(serviceID string, rmVolumes bool) error {
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

		err := db.txSetIPs(tx, ips)
		if err != nil {
			return err
		}

		err = db.txSetPorts(tx, ports)
		if err != nil {
			return err
		}

		for i := range units {
			if rmVolumes {
				err = db.txDelVolumeByUnit(tx, units[i].ID)
				if err != nil {
					return err
				}
			}

			//			err = db.txDelUnitConfigByUnit(tx, units[i].ID)
			//			if err != nil {
			//				return err
			//			}
		}

		//		err = db.txDelBackupStrategy(tx, serviceID)
		//		if err != nil {
		//			return err
		//		}

		err = db.txDelUsers(tx, serviceID)
		if err != nil {
			return err
		}

		err = db.txDelUnit(tx, serviceID)
		if err != nil {
			return err
		}

		err = db.txDelService(tx, serviceID)

		return err
	}

	return db.txFrame(do)
}

type ServiceInfo struct {
	Service Service
	Users   []User
	Units   []UnitInfo
}

type UnitInfo struct {
	Unit        Unit
	Engine      Node
	Volumes     []Volume
	Networkings []IP
}

func (db dbBase) GetServiceInfo(nameOrID string) (info ServiceInfo, err error) {
	svc, err := db.GetService(nameOrID)
	if err != nil {
		return
	}

	info.Service = svc

	users, err := db.ListUsersByService(svc.ID, "")
	if err != nil {
		return
	}

	info.Users = users

	units, err := db.ListUnitByServiceID(svc.ID)
	if err != nil {
		return
	}

	list := make([]UnitInfo, len(units))

	for i := range units {
		lvs, err := db.ListVolumesByUnitID(units[i].ID)
		if err != nil {
			return info, err
		}

		node, err := db.GetNode(units[i].EngineID)
		if err != nil {
			return info, err
		}

		ips, err := db.ListIPByUnitID(units[i].ID)
		if err != nil {
			return info, err
		}

		list[i] = UnitInfo{
			Unit:        units[i],
			Engine:      node,
			Volumes:     lvs,
			Networkings: ips,
		}
	}

	info.Units = list

	return info, nil
}

func (db dbBase) ListServicesInfo() ([]ServiceInfo, error) {
	services, err := db.ListServices()
	if err != nil {
		return nil, err
	}

	nodes, err := db.ListNodes()
	if err != nil {
		return nil, err
	}

	units, err := db.listUnits()
	if err != nil {
		return nil, err
	}

	users, err := db.listUsers()
	if err != nil {
		return nil, err
	}

	volumes, err := db.listVolumes()
	if err != nil {
		return nil, err
	}

	ips, err := db.listIPsByAllocated(true, 0)
	if err != nil {
		return nil, err
	}

	list := make([]ServiceInfo, 0, len(services))
	for i := range services {
		us := make([]User, 0, 5)
		unitsInfo := make([]UnitInfo, 0, 5)

		for u := range users {
			if users[u].ServiceID == services[i].ID {
				us = append(us, users[u])
			}
		}

		for u := range units {
			if units[u].ServiceID != services[i].ID {
				continue
			}

			node := Node{}
			if units[u].EngineID != "" {
				for n := range nodes {
					if nodes[n].EngineID == units[u].EngineID {
						node = nodes[n]
					}
				}
			}

			lvs := make([]Volume, 0, 2)
			for v := range volumes {
				if volumes[v].UnitID == units[u].ID {
					lvs = append(lvs, volumes[v])
				}
			}

			nets := make([]IP, 0, 2)
			for n := range ips {
				if ips[n].UnitID == units[u].ID {
					nets = append(nets, ips[n])
				}
			}

			unitsInfo = append(unitsInfo, UnitInfo{
				Unit:        units[u],
				Engine:      node,
				Volumes:     lvs,
				Networkings: nets,
			})
		}

		list = append(list, ServiceInfo{
			Service: services[i],
			Users:   us,
			Units:   unitsInfo,
		})
	}

	return list, nil
}

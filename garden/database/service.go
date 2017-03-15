package database

import (
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type ServiceInterface interface {
	InsertService(svc Service, units []Unit, t *Task) error

	GetService(nameOrID string) (Service, error)
	GetServiceStatus(nameOrID string) (int, error)
	GetServiceByUnit(nameOrID string) (Service, error)

	ListServices() ([]Service, error)

	SetServiceStatus(nameOrID string, val int, finish time.Time) error
	// SetServcieDesc(id, desc string, size int) error
	ServiceStatusCAS(nameOrID string, val int, finish time.Time, f func(val int) bool) (bool, int, error)
	SetServiceWithTask(svc *Service, t Task, state int, finish time.Time) error
}

type ServiceInfoInterface interface {
	GetServiceInfo(nameOrID string) (ServiceInfo, error)
	ListServicesInfo() ([]ServiceInfo, error)

	DelServiceRelation(serviceID string, rmVolumes bool) error
}

type ServiceOrmer interface {
	ServiceInterface

	ServiceInfoInterface

	UnitInterface

	ContainerInterface

	NodeInterface

	VolumeOrmer

	NetworkingOrmer

	ImageInterface

	TaskOrmer

	SysConfigOrmer
}

type Service struct {
	service
	Desc *ServiceDesc
}

// Service if table structure
type service struct {
	ID                string `db:"id" json:"id"`
	Name              string `db:"name" json:"name"`
	DescID            string `db:"description_id" json:"description_id"` // short for Description
	Tag               string `db:"tag" json:"tag"`                       // part of business
	AutoHealing       bool   `db:"auto_healing" json:"auto_healing"`
	AutoScaling       bool   `db:"auto_scaling" json:"auto_scaling"`
	HighAvailable     bool   `db:"high_available" json:"high_available"`
	Status            int    `db:"action_status" json:"action_status"`
	BackupMaxSizeByte int    `db:"backup_max_size" json:"backup_max_size"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	BackupFilesRetention int       `db:"backup_files_retention" json:"backup_files_retention"`
	CreatedAt            time.Time `db:"created_at" json:"created_at"`
	FinishedAt           time.Time `db:"finished_at" json:"finished_at"`
}

func (s Service) ParseImage() (string, string) {
	if s.Desc == nil {
		return "", ""
	}
	parts := strings.SplitN(s.Desc.Image, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return s.Desc.Image, ""
}

func (db dbBase) serviceTable() string {
	return db.prefix + "_service"
}

// ListServices returns all []Service
func (db dbBase) ListServices() ([]Service, error) {
	var (
		out   []Service
		query = "SELECT id,name,description_id,tag,auto_healing,auto_scaling,high_available,action_status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable()
	)

	err := db.Select(&out, query)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, nil
		}

		return nil, errors.Wrap(err, "list []Service")
	}

	list, err := db.listServiceDescs()
	if err != nil {
		return nil, err
	}

	for i := range out {
	in:
		for l := range list {
			if out[i].DescID == list[l].ID {
				out[i].Desc = &list[l]
				break in
			}
		}
	}

	return out, errors.Wrap(err, "list []Service")
}

// GetService returns Service select by ID or Name
func (db dbBase) GetService(nameOrID string) (Service, error) {
	var (
		s     = Service{}
		query = "SELECT id,name,description_id,tag,auto_healing,auto_scaling,high_available,action_status,backup_max_size,backup_files_retention,created_at,finished_at FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	)

	err := db.Get(&s, query, nameOrID, nameOrID)
	if err != nil {
		return s, errors.Wrap(err, "get Service by nameOrID")
	}

	desc, err := db.getServiceDesc(s.DescID)
	if err != nil {
		return s, err
	}

	s.Desc = &desc

	return s, errors.Wrap(err, "get Service by nameOrID")
}

// GetServiceStatus returns Service Status select by ID or Name
func (db dbBase) GetServiceStatus(nameOrID string) (int, error) {
	var (
		n     int
		query = "SELECT action_status FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	)

	err := db.Get(&n, query, nameOrID, nameOrID)

	return n, errors.Wrap(err, "get Service.Status by nameOrID")
}

// GetServiceByUnit returns Service select by Unit ID or Name.
func (db dbBase) GetServiceByUnit(nameOrID string) (Service, error) {

	var (
		id    string
		query = "SELECT service_id FROM " + db.unitTable() + " WHERE id=? OR name=?"
	)

	err := db.Get(&id, query, nameOrID, nameOrID)
	if err != nil {
		return Service{}, errors.Wrap(err, "get Unit")
	}

	service, err := db.GetService(id)

	return service, errors.Wrap(err, "get Service by unit")
}

// SetServiceBackupSize update Service BackupMaxSizeByte
func (db dbBase) SetServiceBackupSize(id string, size int) (err error) {

	query := "UPDATE " + db.serviceTable() + " SET backup_max_size=? WHERE id=?"
	_, err = db.Exec(query, size, id)

	return errors.Wrap(err, "update Service.Desc")
}

func (db dbBase) ServiceStatusCAS(nameOrID string, val int, finish time.Time, f func(val int) bool) (bool, int, error) {
	var (
		status int
		done   bool
	)

	do := func(tx *sqlx.Tx) error {

		query := "SELECT action_status FROM " + db.serviceTable() + " WHERE id=? OR name=?"

		err := tx.Get(&status, query, nameOrID, nameOrID)
		if err != nil {
			return errors.Wrap(err, "Tx get Service Status")
		}

		if !f(status) {
			return nil
		}

		query = "UPDATE " + db.serviceTable() + " SET action_status=?,finished_at=? WHERE id=? OR name=?"

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
func (db dbBase) InsertService(svc Service, units []Unit, t *Task) error {
	do := func(tx *sqlx.Tx) error {

		if svc.Desc != nil {
			err := db.txInsertSerivceDesc(tx, svc.Desc)
			if err != nil {
				return err
			}

			svc.DescID = svc.Desc.ID
		}

		err := db.txInsertSerivce(tx, svc.service)
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
			err = db.txInsertTask(tx, *t, db.serviceTable())
			if err != nil {
				return err
			}
		}

		return err
	}

	return db.txFrame(do)
}

func (db dbBase) txInsertSerivce(tx *sqlx.Tx, svc service) error {

	query := "INSERT INTO " + db.serviceTable() + " ( id,name,description_id,tag,auto_healing,auto_scaling,high_available,action_status,backup_max_size,backup_files_retention,created_at,finished_at ) VALUES ( :id,:name,:description_id,:tag,:auto_healing,:auto_scaling,:high_available,:action_status,:backup_max_size,:backup_files_retention,:created_at,:finished_at )"

	_, err := tx.NamedExec(query, &svc)

	return errors.Wrap(err, "Tx insert Service")
}

// SetServiceStatus update Service Status
func (db dbBase) SetServiceStatus(nameOrID string, state int, finish time.Time) error {
	if finish.IsZero() {

		query := "UPDATE " + db.serviceTable() + " SET action_status=? WHERE id=? OR name=?"
		_, err := db.Exec(query, state, nameOrID, nameOrID)

		return errors.Wrap(err, "update Service Status")
	}

	query := "UPDATE " + db.serviceTable() + " SET action_status=?,finished_at=? WHERE id=? OR name=?"
	_, err := db.Exec(query, state, finish, nameOrID, nameOrID)

	return errors.Wrap(err, "update Service Status & FinishedAt")
}

// SetServiceWithTask update Service Status and Task Status in Tx.
func (db dbBase) SetServiceWithTask(svc *Service, t Task, state int, finish time.Time) error {
	do := func(tx *sqlx.Tx) (err error) {

		if finish.IsZero() {

			query := "UPDATE " + db.serviceTable() + " SET action_status=? WHERE id=?"
			_, err = tx.Exec(query, state, svc.ID)

		} else {

			query := "UPDATE " + db.serviceTable() + " SET action_status=?,finished_at=? WHERE id=?"
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

	return errors.Wrap(err, "tx del Service by nameOrID")
}

// DelServiceRelation delelte related record about Service,
// include Service,Unit,BackupStrategy,IP,Port,LocalVolume,UnitConfig.
// delete in a Tx
func (db dbBase) DelServiceRelation(serviceID string, rmVolumes bool) error {
	units, err := db.ListUnitByServiceID(serviceID)
	if err != nil {
		return err
	}

	// recycle networking & volumes
	ips := make([]IP, 0, 20)
	volumes := make([]Volume, 0, 20)

	for i := range units {

		ipl, err := db.ListIPByUnitID(units[i].ID)
		if err == nil {
			ips = append(ips, ipl...)
		}

		vl, err := db.ListVolumesByUnitID(units[i].ID)
		if err == nil {
			volumes = append(volumes, vl...)
		}

	}

	for i := range ips {
		ips[i].UnitID = ""
		ips[i].Engine = ""
		ips[i].Bandwidth = 0
		ips[i].Bond = ""
	}

	do := func(tx *sqlx.Tx) error {

		err := db.txSetIPs(tx, ips)
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

		err = db.txDelUnit(tx, serviceID)
		if err != nil {
			return err
		}

		err = db.txDelService(tx, serviceID)
		if err != nil {
			return err
		}

		err = db.txDelDescByService(tx, serviceID)

		return err
	}

	return db.txFrame(do)
}

type ServiceInfo struct {
	Service Service
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

		unitsInfo := make([]UnitInfo, 0, 5)

		for u := range units {
			if units[u].ServiceID != services[i].ID {
				continue
			}

			node := Node{}
			if units[u].EngineID != "" {
				for n := range nodes {
					if nodes[n].EngineID == units[u].EngineID {
						node = nodes[n]
						break
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
			Units:   unitsInfo,
		})
	}

	return list, nil
}

type ServiceDesc struct {
	ID           string `db:"id"`
	ServiceID    string `db:"service_id"`
	Architecture string `db:"architecture"`
	Replicas     int    `db:"unit_num"`
	NCPU         int    `db:"cpu_num"`
	Memory       int    `db:"mem_size"`
	Image        string `db:"image_version"`
	Volumes      string `db:"volumes"`
	Networks     string `db:"networks"`
	Clusters     string `db:"cluster_id"`
	Previous     string `db:"previous_version"`
}

func (db dbBase) serviceDescTable() string {
	return db.prefix + "_service_decription"
}

func (db dbBase) getServiceDesc(ID string) (ServiceDesc, error) {
	var (
		s     = ServiceDesc{}
		query = "SELECT id,service_id,architecture,unit_num,cpu_num,mem_size,image_version,volumes,networks,cluster_id,previous_version FROM " + db.serviceDescTable() + " WHERE id=?"
	)

	err := db.Get(&s, query, ID)

	return s, errors.Wrap(err, "get ServiceDesc by ID")
}

// listServiceDescs returns all []ServiceDesc
func (db dbBase) listServiceDescs() ([]ServiceDesc, error) {
	var (
		out   []ServiceDesc
		query = "SELECT id,service_id,architecture,unit_num,cpu_num,mem_size,image_version,volumes,networks,cluster_id,previous_version FROM " + db.serviceDescTable()
	)

	err := db.Select(&out, query)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []ServiceDesc")
}

// listServiceDescs returns all []ServiceDesc
func (db dbBase) listDescByService(ID string) ([]ServiceDesc, error) {
	var (
		out   []ServiceDesc
		query = "SELECT id,service_id,architecture,unit_num,cpu_num,mem_size,image_version,volumes,networks,cluster_id,previous_version FROM " + db.serviceDescTable() + " WHERE service_id=?"
	)

	err := db.Select(&out, query, ID)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []ServiceDesc by serviceID")
}

func (db dbBase) txInsertSerivceDesc(tx *sqlx.Tx, desc *ServiceDesc) error {

	query := "INSERT INTO " + db.serviceDescTable() + " ( id,service_id,architecture,unit_num,cpu_num,mem_size,image_version,volumes,networks,cluster_id,previous_version ) VALUES ( :id,:service_id,:architecture,:unit_num,:cpu_num,:mem_size,:image_version,:volumes,:networks,:cluster_id,:previous_version )"

	_, err := tx.NamedExec(query, desc)

	return errors.Wrap(err, "Tx insert ServiceDesc")
}

func (db dbBase) txDelDescByService(tx *sqlx.Tx, service string) error {
	query := "DELETE FROM " + db.serviceDescTable() + " WHERE service_id=?"
	_, err := tx.Exec(query, service)

	return errors.Wrap(err, "tx del ServiceDesc by serviceID")
}

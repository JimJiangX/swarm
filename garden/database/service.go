package database

import (
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type ServiceIface interface {
	InsertService(svc Service, units []Unit, t *Task) error

	GetService(nameOrID string) (Service, error)
	GetServiceStatus(nameOrID string) (int, error)
	GetServiceByUnit(nameOrID string) (Service, error)

	ListServices() ([]Service, error)

	SetServiceStatus(nameOrID string, val int, finish time.Time) error
	ServiceStatusCAS(nameOrID string, val int, t *Task, f func(val int) bool) (bool, int, error)
	SetServiceWithTask(nameOrID string, val int, t *Task, finish time.Time) error

	SetServiceDesc(svc Service) error
}

type ServiceInfoIface interface {
	GetServiceInfo(nameOrID string) (ServiceInfo, error)
	ListServicesInfo() ([]ServiceInfo, error)

	DelServiceRelation(serviceID string, rmVolumes bool) error
	RecycleResource(ips []IP, lvs []Volume) error
}

type ServiceOrmer interface {
	ServiceIface

	ServiceInfoIface

	UnitIface

	ContainerIface

	NodeIface

	VolumeOrmer

	NetworkingOrmer

	ImageIface

	TaskOrmer

	SysConfigOrmer
}

// Service if table structure
type Service struct {
	Desc *ServiceDesc

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
		found := false
	in:
		for l := range list {
			if out[i].DescID == list[l].ID {
				out[i].Desc = &list[l]
				found = true
				break in
			}
		}

		if !found {
			out[i].Desc = &ServiceDesc{}
		}
	}

	return out, nil
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

	if s.DescID == "" {
		s.Desc = &ServiceDesc{}
	} else {
		desc, err := db.getServiceDesc(s.DescID)
		if err != nil {
			return s, err
		}

		s.Desc = &desc
	}

	return s, nil
}

// GetServiceStatus returns Service Status select by ID or Name
func (db dbBase) GetServiceStatus(nameOrID string) (int, error) {
	var (
		n     int
		query = "SELECT action_status FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	)

	err := db.Get(&n, query, nameOrID, nameOrID)
	if err == nil {
		return n, nil
	}

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

	return db.GetService(id)
}

// SetServiceBackupSize update Service BackupMaxSizeByte
func (db dbBase) SetServiceBackupSize(id string, size int) error {

	query := "UPDATE " + db.serviceTable() + " SET backup_max_size=? WHERE id=?"
	_, err := db.Exec(query, size, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "update Service.Desc")
}

func (db dbBase) ServiceStatusCAS(nameOrID string, val int, t *Task, f func(val int) bool) (bool, int, error) {
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

		query = "UPDATE " + db.serviceTable() + " SET action_status=? WHERE id=? OR name=?"

		_, err = tx.Exec(query, val, nameOrID, nameOrID)
		if err != nil {
			return errors.Wrap(err, "Tx update Service status")
		}

		if t != nil {
			err = db.txInsertTask(tx, *t, db.serviceTable())
			if err != nil {
				return err
			}
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
			err = db.txInsertTask(tx, *t, db.serviceTable())
			if err != nil {
				return err
			}
		}

		return err
	}

	return db.txFrame(do)
}

func (db dbBase) txInsertSerivce(tx *sqlx.Tx, svc Service) error {

	query := "INSERT INTO " + db.serviceTable() + " ( id,name,description_id,tag,auto_healing,auto_scaling,high_available,action_status,backup_max_size,backup_files_retention,created_at,finished_at ) VALUES ( :id,:name,:description_id,:tag,:auto_healing,:auto_scaling,:high_available,:action_status,:backup_max_size,:backup_files_retention,:created_at,:finished_at )"

	_, err := tx.NamedExec(query, &svc)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx insert Service")
}

// SetServiceStatus update Service Status
func (db dbBase) SetServiceStatus(nameOrID string, state int, finish time.Time) error {
	if finish.IsZero() {

		query := "UPDATE " + db.serviceTable() + " SET action_status=? WHERE id=? OR name=?"
		_, err := db.Exec(query, state, nameOrID, nameOrID)
		if err == nil {
			return nil
		}

		return errors.Wrap(err, "update Service Status")
	}

	query := "UPDATE " + db.serviceTable() + " SET action_status=?,finished_at=? WHERE id=? OR name=?"
	_, err := db.Exec(query, state, finish, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "update Service Status & FinishedAt")
}

func (db dbBase) txSetServiceStatus(tx *sqlx.Tx, nameOrID string, status int, now time.Time) (err error) {
	if now.IsZero() {

		query := "UPDATE " + db.serviceTable() + " SET action_status=? WHERE id=? OR name=?"
		_, err = tx.Exec(query, status, nameOrID, nameOrID)
	} else {
		query := "UPDATE " + db.serviceTable() + " SET action_status=?,finished_at=? WHERE id=? OR name=?"
		_, err = tx.Exec(query, status, now, nameOrID, nameOrID)
	}

	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx update Service status")
}

// SetServiceWithTask update Service Status and Task Status in Tx.
func (db dbBase) SetServiceWithTask(nameOrID string, val int, t *Task, finish time.Time) error {
	do := func(tx *sqlx.Tx) error {
		err := db.txSetServiceStatus(tx, nameOrID, val, finish)
		if err != nil {
			return err
		}

		if t != nil {
			err = db.txSetTask(tx, *t)
		}

		return err
	}

	return db.txFrame(do)
}

func (db dbBase) SetServiceScale(svc Service, t Task) error {
	do := func(tx *sqlx.Tx) error {

		now := time.Now()
		t.FinishedAt = now

		err := db.txSetServiceStatus(tx, svc.ID, svc.Status, now)
		if err != nil {
			return err
		}

		err = db.txSetTask(tx, t)
		if err != nil {
			return err
		}

		query := "UPDATE " + db.serviceDescTable() + " SET architecture=?,unit_num=? WHERE id=?"
		_, err = tx.Exec(query, svc.Desc.Architecture, svc.Desc.Replicas, svc.Desc.ID)
		if err == nil {
			return nil
		}

		return errors.Wrap(err, "tx update serviceDesc replicas")
	}

	return db.txFrame(do)
}

func (db dbBase) txDelService(tx *sqlx.Tx, nameOrID string) error {

	query := "DELETE FROM " + db.serviceTable() + " WHERE id=? OR name=?"
	_, err := tx.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

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

	do := func(tx *sqlx.Tx) error {

		err := db.txResetIPs(tx, ips)
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
		}

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

func (db dbBase) RecycleResource(ips []IP, lvs []Volume) error {

	do := func(tx *sqlx.Tx) error {
		err := db.txResetIPs(tx, ips)
		if err != nil {
			return err
		}

		for i := range lvs {
			err = db.txDelVolume(tx, lvs[i].ID)
			if err != nil {
				return err
			}
		}

		return nil
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
	Memory       int64  `db:"mem_size"`
	ImageID      string `db:"image_id"`
	Image        string `db:"image_version"`
	Volumes      string `db:"volumes"`
	Networks     string `db:"networks"`
	Clusters     string `db:"cluster_id"`
	Options      string `db:"options"`
	Previous     string `db:"previous_version"`
}

func (db dbBase) serviceDescTable() string {
	return db.prefix + "_service_decription"
}

func (db dbBase) getServiceDesc(ID string) (ServiceDesc, error) {
	var (
		s     = ServiceDesc{}
		query = "SELECT id,service_id,architecture,unit_num,cpu_num,mem_size,image_id,image_version,volumes,networks,cluster_id,options,previous_version FROM " + db.serviceDescTable() + " WHERE id=?"
	)

	err := db.Get(&s, query, ID)
	if err == nil {
		return s, nil
	}

	return s, errors.Wrap(err, "get ServiceDesc by ID")
}

// listServiceDescs returns all []ServiceDesc
func (db dbBase) listServiceDescs() ([]ServiceDesc, error) {
	var (
		out   []ServiceDesc
		query = "SELECT id,service_id,architecture,unit_num,cpu_num,mem_size,image_id,image_version,volumes,networks,cluster_id,options,previous_version FROM " + db.serviceDescTable()
	)

	err := db.Select(&out, query)
	if err == nil {
		return out, nil
	} else if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, errors.Wrap(err, "list []ServiceDesc")
}

// listServiceDescs returns all []ServiceDesc
func (db dbBase) listDescByService(ID string) ([]ServiceDesc, error) {
	var (
		out   []ServiceDesc
		query = "SELECT id,service_id,architecture,unit_num,cpu_num,mem_size,image_id,image_version,volumes,networks,cluster_id,options,previous_version FROM " + db.serviceDescTable() + " WHERE service_id=?"
	)

	err := db.Select(&out, query, ID)
	if err == nil {
		return out, nil
	} else if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, errors.Wrap(err, "list []ServiceDesc by serviceID")
}

func (db dbBase) txInsertSerivceDesc(tx *sqlx.Tx, desc *ServiceDesc) error {

	query := "INSERT INTO " + db.serviceDescTable() + " ( id,service_id,architecture,unit_num,cpu_num,mem_size,image_id,image_version,volumes,networks,cluster_id,options,previous_version ) VALUES ( :id,:service_id,:architecture,:unit_num,:cpu_num,:mem_size,:image_id,:image_version,:volumes,:networks,:cluster_id,:options,:previous_version )"

	_, err := tx.NamedExec(query, desc)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx insert ServiceDesc")
}

func (db dbBase) txDelDescByService(tx *sqlx.Tx, service string) error {
	query := "DELETE FROM " + db.serviceDescTable() + " WHERE service_id=?"
	_, err := tx.Exec(query, service)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "tx del ServiceDesc by serviceID")
}

func (db dbBase) SetServiceDesc(svc Service) error {
	do := func(tx *sqlx.Tx) error {
		query := "UPDATE " + db.serviceTable() + " SET description_id=? WHERE id=?"

		_, err := tx.Exec(query, svc.DescID, svc.ID)
		if err != nil {
			return errors.Wrap(err, "update Service DescID")
		}

		return db.txInsertSerivceDesc(tx, svc.Desc)
	}

	return db.txFrame(do)
}

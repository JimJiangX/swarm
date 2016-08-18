package database

import (
	"fmt"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertIPQuery = "INSERT INTO tb_ip (ip_addr,prefix,networking_id,unit_id,allocated) VALUES (:ip_addr,:prefix,:networking_id,:unit_id,:allocated)"

type IP struct {
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       int    `db:"prefix"`
	NetworkingID string `db:"networking_id"`
	UnitID       string `db:"unit_id"`
	Allocated    bool   `db:"allocated"`
}

func (ip IP) TableName() string {
	return "tb_ip"
}

const insertNetworkingQuery = "INSERT INTO tb_networking (id,type,gateway,enabled) VALUES (:id,:type,:gateway,:enabled)"

type Networking struct {
	ID      string `db:"id"`
	Type    string `db:"type"`
	Gateway string `db:"gateway"`
	Enabled bool   `db:"enabled"`
}

func (net Networking) TableName() string {
	return "tb_networking"
}

const insertPortQuery = "INSERT INTO tb_port (port,name,unit_id,unit_name,proto,allocated) VALUES (:port,:name,:unit_id,:unit_name,:proto,:allocated)"

type Port struct {
	Port      int    `db:"port"`
	Name      string `db:"name"`
	UnitID    string `db:"unit_id" json:"-"`
	UnitName  string `db:"unit_name"`
	Proto     string `db:"proto"` // tcp/udp
	Allocated bool   `db:"allocated" json:"-"`
}

func (port Port) TableName() string {
	return "tb_port"
}

func NewPort(port int, name, unitID, unitName, proto string, allocated bool) Port {
	return Port{
		Port:      port,
		Name:      name,
		UnitID:    unitID,
		UnitName:  unitName,
		Proto:     proto,
		Allocated: allocated,
	}
}

// Delete Port only if Port.Allocated==allocated
func DeletePort(port int, allocated bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	using := false
	err = db.Get(&using, "SELECT allocated FROM tb_port WHERE port=?", port)
	if err != nil {
		return errors.Wrap(err, "Get Port.Allocated")
	}
	if using != allocated {
		return errors.Errorf("Port %d (%t!= %t),cannot be removed", port, using, allocated)
	}

	_, err = db.Exec("DELETE FROM tb_port WHERE port=? AND allocated=?", port, allocated)
	if err != nil {
		return errors.Wrap(err, "Delete Port")
	}

	return err
}

func SelectAvailablePorts(num int) ([]Port, error) {
	if num == 0 {
		return nil, nil
	}

	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var (
		ports []Port
		query = fmt.Sprintf("SELECT * FROM tb_port WHERE allocated=? LIMIT %d", num)
	)

	err = db.Select(&ports, query, false)
	if err != nil {
		db, err = GetDB(true)
		if err != nil {
			return nil, err
		}

		err = db.Select(&ports, query, false)
		if err != nil {
			return nil, errors.Wrap(err, "Select []Port")
		}
	}

	if len(ports) != num {
		return nil, errors.Errorf("Cannot get required num=%d available ports", num)
	}

	return ports, nil
}

func TxUpdatePorts(tx *sqlx.Tx, ports []Port) error {
	const query = "UPDATE tb_port SET name=:name,unit_id=:unit_id,unit_name=:unit_name,proto=:proto,allocated=:allocated WHERE port=:port"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx Prepare Update []Port")
	}

	for i := range ports {
		_, err = stmt.Exec(&ports[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx Update Port")
		}
	}

	err = stmt.Close()
	if err != nil {
		return errors.Wrap(err, "Tx Update []Port")
	}

	return nil
}

// TxImportPort import Port from start to end(includ end)
func TxImportPort(start, end int, filter ...int) (int, error) {
	ports := make([]Port, 0, end-start+1)

loop:
	for i := start; i <= end; i++ {
		for _, val := range filter {
			if i == val {
				continue loop
			}
		}

		ports = append(ports, Port{
			Port:      i,
			Allocated: false,
		})
	}

	tx, err := GetTX()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	err = txInsertPorts(tx, ports)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, errors.Wrap(err, "Insert []Port Commit")
	}

	return len(ports), nil
}

func txInsertPorts(tx *sqlx.Tx, ports []Port) error {
	stmt, err := tx.PrepareNamed(insertPortQuery)
	if err != nil {
		return errors.Wrap(err, "Tx Prepare Insert Port")
	}

	for i := range ports {
		_, err = stmt.Exec(&ports[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx Insert Port")
		}
	}

	err = stmt.Close()
	if err != nil {
		return errors.Wrap(err, "Tx Insert []Port")
	}

	return nil
}

func ListPortsByUnit(nameOrID string) ([]Port, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var ports []Port
	const query = "SELECT * FROM tb_port WHERE unit_id=? OR unit_name=?"

	err = db.Select(&ports, query, nameOrID, nameOrID)
	if err == nil {
		return ports, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&ports, query, nameOrID, nameOrID)
	if err == nil {
		return ports, nil
	}

	return nil, errors.Wrap(err, "Select []Port")
}

func ListPorts(start, end, limit int) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	ports := make([]Port, 0, limit)

	switch {
	case start == 0 && end == 0:
		query := fmt.Sprintf("SELECT * FROM tb_port LIMIT %d", limit)
		err = db.Select(&ports, query)

	case start > 0 && end > 0:
		query := fmt.Sprintf("SELECT * FROM tb_port WHERE port>=? AND port<=? LIMIT %d", limit)
		err = db.Select(&ports, query, start, end)

	case end == 0:
		query := fmt.Sprintf("SELECT * FROM tb_port WHERE port>=? LIMIT %d", limit)
		err = db.Select(&ports, query, start)

	case start == 0:
		query := fmt.Sprintf("SELECT * FROM tb_port WHERE port<=? LIMIT %d", limit)
		err = db.Select(&ports, query, end)

	default:
		return nil, errors.Errorf("illegal input,start=%d end=%d limit=%d", start, end, limit)
	}

	if err != nil {
		return nil, errors.Wrap(err, "Select []Port")
	}

	return ports, nil
}

func GetNetworkingByID(id string) (Networking, int, error) {
	db, err := GetDB(true)
	if err != nil {
		return Networking{}, 0, err
	}

	net := Networking{}

	err = db.Get(&net, "SELECT * FROM tb_networking WHERE id=?", id)
	if err != nil {
		return net, 0, errors.Wrap(err, "Get Networking")
	}

	prefix := 0
	err = db.Get(&prefix, "SELECT prefix FROM tb_ip WHERE networking_id=? LIMIT 1", id)
	if err != nil {
		return Networking{}, 0, errors.Wrap(err, "Get IP.Prefix")
	}

	return net, prefix, nil
}

func ListIPByUnitID(unit string) ([]IP, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var out []IP
	const query = "SELECT * from tb_ip WHERE unit_id=?"

	err = db.Select(&out, query, unit)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, unit)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "Select []IP")
}

func ListNetworkingByType(_type string) ([]Networking, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []Networking
	const query = "SELECT * FROM tb_networking WHERE type=?"

	err = db.Select(&list, query, _type)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, _type)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "Select []Networking")
}

func ListNetworking() ([]Networking, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []Networking
	const query = "SELECT * FROM tb_networking"

	err = db.Select(&list, query)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "Select []Networking")
}

func ListIPByNetworking(id string) ([]IP, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []IP
	const query = "SELECT * FROM tb_ip WHERE networking_id=?"

	err = db.Select(&list, query, id)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, id)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "Select []IP")
}

func TxInsertNetworking(start, end, gateway, _type string, prefix int) (Networking, []IP, error) {
	startU32 := utils.IPToUint32(start)
	endU32 := utils.IPToUint32(end)
	if move := uint(32 - prefix); (startU32 >> move) != (endU32 >> move) {
		return Networking{}, nil, errors.Errorf("%s-%s is different network segments", start, end)
	}
	if startU32 > endU32 {
		startU32, endU32 = endU32, startU32
	}
	net := Networking{
		ID:      utils.Generate64UUID(),
		Type:    _type,
		Gateway: gateway,
		Enabled: true,
	}

	num := int(endU32 - startU32 + 1)
	ips := make([]IP, num)
	for i := range ips {
		ips[i] = IP{
			IPAddr:       startU32,
			Prefix:       prefix,
			NetworkingID: net.ID,
			Allocated:    false,
		}

		fmt.Println(i, startU32, utils.Uint32ToIP(startU32).String())

		startU32++
	}

	// insert to database
	err := txInsertNetworking(net, ips)
	if err != nil {
		return Networking{}, nil, err
	}

	return net, ips, nil
}

func UpdateNetworkingStatus(id string, enable bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_networking SET enabled=? WHERE id=?"

	_, err = db.Exec(query, enable, id)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, enable, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Update Networking")
}

func txInsertNetworking(net Networking, ips []IP) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareNamed(insertIPQuery)
	if err != nil {
		return errors.Wrap(err, "Tx Prepare Insert []IP")
	}

	for i := range ips {
		_, err = stmt.Exec(&ips[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx Insert IP")
		}
	}

	stmt.Close()

	_, err = tx.NamedExec(insertNetworkingQuery, &net)
	if err != nil {
		return errors.Wrap(err, "Tx Insert Networking")
	}

	return tx.Commit()
}

func TxDeleteNetworking(id string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM tb_networking WHERE id=?", id)
	if err != nil {
		return errors.Wrap(err, "Delete Networking")
	}

	_, err = tx.Exec("DELETE FROM tb_ip WHERE networking_id=?", id)
	if err != nil {
		return errors.Wrap(err, "Delete []IP")
	}

	return tx.Commit()
}

func CountIPByNetwrokingAndStatus(networking string, allocation bool) (int, error) {
	db, err := GetDB(false)
	if err != nil {
		return 0, err
	}

	count := 0
	const query = "SELECT COUNT(id) from tb_ip WHERE networking_id=? AND allocated=?"

	err = db.Get(&count, query, networking, allocation)
	if err == nil {
		return count, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return 0, err
	}

	err = db.Get(&count, query, networking, allocation)
	if err == nil {
		return count, nil
	}

	return 0, errors.Wrap(err, "Count Networking.IP")
}

func GetMultiIPByNetworking(networking string, allocation bool, num int) ([]IP, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var out []IP
	query := fmt.Sprintf("SELECT * from tb_ip WHERE networking_id=? AND allocated=? LIMIT %d", num)

	err = db.Select(&out, query, networking, allocation)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, networking, allocation)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "Select []IP")
}

func TXAllocIPByNetworking(id, unit string) (IP, error) {
	tx, err := GetTX()
	if err != nil {
		return IP{}, err
	}
	defer tx.Rollback()

	out := IP{}
	query := tx.Rebind(fmt.Sprintf("SELECT * FROM tb_ip WHERE networking_id=? AND allocated=? LIMIT 1 FOR UPDATE;"))

	err = tx.Get(&out, query, id, false)
	if err != nil {
		return out, errors.Wrap(err, "Tx Get IP")
	}

	out.Allocated = true
	out.UnitID = unit

	query = "UPDATE tb_ip SET allocated=:allocated,unit_id=:unit_id WHERE ip_addr=:ip_addr AND prefix=:prefix"

	_, err = tx.NamedExec(query, out)
	if err != nil {
		return out, errors.Wrap(err, "Update IP")
	}

	if err = tx.Commit(); err != nil {
		return out, errors.Wrap(err, "Update IP Commit")
	}

	return out, nil
}

func TxUpdateMultiIPValue(tx *sqlx.Tx, val []IP) error {
	const query = "UPDATE tb_ip SET unit_id=:unit_id,allocated=:allocated WHERE ip_addr=:ip_addr AND prefix=:prefix"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx Prepare Update []IP")
	}

	for i := range val {
		_, err = stmt.Exec(&val[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx Update IP")
		}
	}

	err = stmt.Close()
	if err != nil {
		return errors.Wrap(err, "Tx Update []IP")
	}

	return nil
}

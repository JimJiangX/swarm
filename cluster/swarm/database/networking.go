package database

import (
	"fmt"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
)

type IP struct {
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       uint32 `db:"prefix"`
	NetworkingID string `db:"networking_id"`
	UnitID       string `db:"unit_id"`
	Allocated    bool   `db:"allocated"`
}

func (ip IP) TableName() string {
	return "tb_ip"
}

type Networking struct {
	ID         string `db:"id"`
	Networking string `db:"networking"`
	Type       string `db:"type"`
	Gateway    string `db"gateway"`
	Enabled    bool   `db:"enabled"`
}

func (net Networking) TableName() string {
	return "tb_networking"
}

type Port struct {
	Port      int    `db:"port"`
	Name      string `db:"name"`
	UnitID    string `db:"unit_id" json:"-"`
	Proto     string `db:"proto"` // tcp/udp
	Allocated bool   `db:"allocated" json:"-"`
}

func (port Port) TableName() string {
	return "tb_port"
}

func NewPort(port int, name, unit, proto string, allocated bool) Port {
	return Port{
		Port:      port,
		Name:      name,
		UnitID:    unit,
		Proto:     proto,
		Allocated: allocated,
	}
}

func SetPortAllocated(port int, allocated bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_port SET allocated=? WHERE port=?", allocated, port)

	return err
}

func SelectAvailablePorts(num int) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var ports []Port
	query := fmt.Sprintf("SELECT * FROM tb_port WHERE allocated=? LIMIT %d", num)

	err = db.Select(&ports, query, false)
	if err != nil {
		return nil, err
	}

	return ports, nil
}

func TxUpdatePorts(tx *sqlx.Tx, ports []Port) error {
	query := "UPDATE tb_port SET name=:name,unit_id=:unit_id,proto=:proto,allocated=:allocated WHERE port=:port"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}

	for i := range ports {
		_, err = stmt.Exec(&ports[i])
		if err != nil {
			return err
		}
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

	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	err = TxInsertPorts(tx, ports)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return len(ports), nil
}

func TxInsertPorts(tx *sqlx.Tx, ports []Port) error {
	query := "INSERT INTO tb_port (port,name,unit_id,proto,allocated) VALUES (:port,:name,:unit_id,:proto,:allocated)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}

	for i := range ports {
		_, err = stmt.Exec(&ports[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func DelMultiPorts(tx *sqlx.Tx, ports []Port) error {
	query := "DELETE FROM tb_port WHERE port=?"

	stmt, err := tx.Preparex(query)
	if err != nil {
		return err
	}

	for i := range ports {
		_, err := stmt.Exec(ports[i].Port)
		if err != nil {
			// return err
		}
	}

	return nil
}

func GetPortsByUnit(id string) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var ports []Port
	err = db.Select(&ports, "SELECT * FROM tb_port WHERE unit_id=?", id)
	if err != nil {
		return nil, err
	}

	return ports, nil
}

func TxInsertNetworking(id, addr, gateway, typ string, prefix, num int) error {
	net := Networking{
		ID:         id,
		Networking: addr,
		Type:       typ,
		Gateway:    gateway,
		Enabled:    true,
	}

	ips := make([]IP, num)
	addrU32 := utils.IPToUint32(net.Networking)
	prefixU32 := uint32(prefix)

	for i := range ips {
		ips[i] = IP{
			IPAddr:       addrU32,
			Prefix:       prefixU32,
			NetworkingID: net.ID,
			Allocated:    false,
		}

		fmt.Println(i, addrU32, utils.Uint32ToIP(addrU32).String())

		addrU32++
	}

	// insert to database

	return insertNetworking(net, ips)
}

func UpdateNetworkingStatus(id string, enable bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE tb_networking SET enabled=? WHERE id=?", enable, id)

	return err
}

func insertNetworking(net Networking, ips []IP) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := "INSERT INTO tb_networking (id,networking,type,gateway,enabled) VALUES (:id,:networking,:type,:gateway,:enabled)"
	_, err = tx.NamedExec(query, &net)
	if err != nil {
		return err
	}

	ipQuery := "INSERT INTO tb_ip (ip_addr,prefix,networking_id,unit_id,allocated) VALUES (:ip_addr,:prefix,:networking_id,:unit_id,:allocated)"
	stmt, err := tx.PrepareNamed(ipQuery)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range ips {
		_, err = stmt.Exec(&ips[i])
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

type IPStatus struct {
	UnitID    string
	IP        uint32
	Prefix    int
	Allocated bool
}

func TxUpdateMultiIPStatue(tx *sqlx.Tx, val []IPStatus) error {
	query := "UPDATE tb_ip SET unit_id=:unit_id,allocated=:allocated WHERE ip=:ip AND prefix=:prefix"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range val {
		_, err = stmt.Exec(&val[i])
		if err != nil {
			return err
		}
	}

	return nil
}

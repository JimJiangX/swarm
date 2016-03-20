package database

import (
	"encoding/binary"
	"net"

	"github.com/jmoiron/sqlx"
)

type IP struct {
	IPAddr       uint32 `db:"ip_addr"`
	Prefix       uint32 `db:"prefix"`
	NetworkingID string `db:"network_id"`
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
	Port      int    `db:"port"` //  自增
	Name      string `db:"name"`
	UnitID    string `db:"unit_id"`
	Allocated bool   `db:"allocated"`
}

func (port Port) TableName() string {
	return "tb_port"
}

func NewPort(port int, name, unit string, allocated bool) Port {
	return Port{
		Port:      port,
		Name:      name,
		UnitID:    unit,
		Allocated: allocated,
	}
}

func TxInsertPorts(tx *sqlx.Tx, ports []Port, allocated bool) error {
	query := "INSERT INTO tb_port (port,name,unit_id,allocated) VALUES (:port,:name,:unit_id,:allocated)"

	stmt, err := tx.Preparex(query)
	if err != nil {
		return err
	}

	for i := range ports {
		result, err := stmt.Exec(&ports[i])
		if err != nil {
			return err
		}

		port, err := result.LastInsertId()
		if err != nil {
			return err
		}

		ports[i].Port = int(port)

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

func GetPortsByUnit(IDOrName string) ([]Port, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var ports []Port
	err = db.QueryRowx("SELECT * From tb_port WHERE unit_id=?", IDOrName).StructScan(&ports)
	if err != nil {
		return nil, err
	}

	return ports, nil
}

func InsertNetworking(id, addr, gateway, typ string, prefix, num int) error {
	net := Networking{
		ID:         id,
		Networking: addr,
		Type:       typ,
		Gateway:    gateway,
		Enabled:    true,
	}

	ips := make([]IP, num)
	addrU32 := IPToUint32(net.Networking)
	prefixU32 := uint32(prefix)

	for i := range ips {
		ips[i] = IP{
			IPAddr:       addrU32,
			Prefix:       prefixU32,
			NetworkingID: net.ID,
			Allocated:    false,
		}

		addrU32++
	}

	// insert to database

	return insertNetworking(net, ips)
}

func insertNetworking(net Networking, ips []IP) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := "INSERT INTO tb_networking (id,networking,type,gateway,enabled) VALUES (:id,:networking,:type,:gateway,:enabled)"
	_, err = tx.NamedExec(query, &net)
	if err != nil {
		return err
	}

	ipQuery := "INSERT INTO tb_ip (ip_addr,prefix,network_id,allocated) VALUES (:ip_addr,:prefix,:network_id,:allocated)"
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
	IP        uint32
	Prefix    int
	Allocated bool
}

func TxUpdateMultiIPStatue(tx *sqlx.Tx, values []IPStatus) error {
	query := "UPDATE tb_ip SET allocated=? WHERE ip=? AND prefix=?"

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range values {

		_, err = stmt.Exec(values[i].Allocated, values[i].IP, values[i].Prefix)
		if err != nil {
			return err
		}
	}

	return nil
}

func IPToUint32(ip string) uint32 {
	addr := net.ParseIP(ip)
	if addr == nil {
		return 0
	}
	return binary.BigEndian.Uint32(addr.To4())
}

func Uint32ToIP(cidr uint32) net.IP {
	addr := make([]byte, 4)
	binary.BigEndian.PutUint32(addr, cidr)
	return net.IP(addr)
}

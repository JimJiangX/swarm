package database

import (
	"math/rand"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestPort(t *testing.T) {
	ports := []Port{
		NewPort(1, "port1", "unit1", "tcp", true),
		NewPort(2, "port2", "unit2", "tcp", false),
		NewPort(3, "port3", "unit3", "udp", true),
		NewPort(4, "port4", "unit4", "udp", false),
	}

	defer func() {
		tx, err := GetTX()
		if err != nil {
			t.Fatal(err)
		}
		err = DelMultiPorts(tx, ports)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit()
		if err != nil {
			t.Fatal(err)
		}
	}()

	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxInsertPorts(tx, ports)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}
	p1, err := SelectAvailablePorts(4)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 2 {
		t.Fatal("Available Ports should be 2", p1)
	}

	err = SetPortAllocated(1, false)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := SelectAvailablePorts(4)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2) != 3 {
		t.Fatal("Available Ports should be 3")
	}

	var p3 []Port
	_p3 := NewPort(3, "port3b", "unit3b", "udp", false)
	p3 = append(p3, _p3)
	tx, err = GetTX()
	err = TxUpdatePorts(tx, p3)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	p4, err := SelectAvailablePorts(4)
	if err != nil {
		t.Fatal(err)
	}
	if len(p4) != 4 {
		t.Fatal("Available Ports should be 4")
	}

	n, err := TxImportPort(5, 10, 7, 8)
	defer func() {
		tx, err := GetTX()
		if err != nil {
			t.Fatal(err)
		}
		err = DelMultiPorts(
			tx,
			[]Port{
				NewPort(5, "", "", "", true),
				NewPort(6, "", "", "", true),
				NewPort(9, "", "", "", true),
				NewPort(10, "", "", "", true),
			})
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit()
		if err != nil {
			t.Fatal(err)
		}
	}()
	p5, err := SelectAvailablePorts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(p5) != 4+n {
		t.Fatal("Available Ports should be 8")
	}

	p6, err := GetPortsByUnit("unit1")
	if err != nil {
		t.Fatal(err)
	}
	if len(p6) != 1 {
		t.Fatal("unit1 Ports should be 1")
	}
}

func TestNetwork(t *testing.T) {
	networking, _, err := TxInsertNetworking("192.168.2.14", "192.168.2.144", "255.255.255.0", "tcp", 24)
	if err != nil {
		t.Fatal(err)
	}

	defer TxDeleteNetworking(networking.ID)

	err = UpdateNetworkingStatus(networking.ID, false)
	if err != nil {
		t.Fatal(err)
	}
}

func randNetworkId() string {
	rand.Seed(time.Now().Unix())
	return "network" + strconv.Itoa(int(rand.Int31n(1000)))
}

func randIp() string {
	rand.Seed(time.Now().Unix())
	ip1 := rand.Int31n(255)
	rand.Seed(int64(ip1))
	ip2 := rand.Int31n(255)
	rand.Seed(int64(ip2))
	ip3 := rand.Int31n(255)
	rand.Seed(int64(ip3))
	ip4 := rand.Int31n(255)
	ip := net.IPv4(byte(ip1), byte(ip2), byte(ip3), byte(ip4))
	return ip.String()
}

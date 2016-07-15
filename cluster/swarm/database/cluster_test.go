package database

import (
	"testing"
	"time"

	"github.com/docker/swarm/utils"
)

func NewCluster(name, _type, storageType, storageID, networking string,
	enable bool, num int, limit float32) Cluster {
	return Cluster{
		ID:           utils.Generate64UUID(),
		Name:         name,
		Type:         _type,
		StorageType:  storageType,
		StorageID:    storageID,
		NetworkingID: networking,
		Enabled:      enable,
		MaxNode:      num,
		UsageLimit:   limit,
	}
}

func TestCluster(t *testing.T) {
	clusters := []Cluster{
		NewCluster("cluster1", "upsql", "local", "", "", true, 256, 0.8),
		NewCluster("cluster2", "upsql", "ssd", "", "", true, 100000000, 1.44),
		NewCluster("cluster3", "proxy", "local", "", "", true, 100000000, 1.44),
		NewCluster("cluster4", "proxy", "local", "", "", false, 100000000, 1.44),
	}
	wrong := []Cluster{
		NewCluster("cluster2", "nomal", "ssd", "", "", false, 1000000000000, 1.44),
		NewCluster("cluster3", "proxy", "local", "", "", false, 100000000, 1.44),
		NewCluster("cluster4", "proxy", "local", "", "", false, 100000000, 1.44),
	}

	for i := range clusters {
		if err := clusters[i].Insert(); err != nil {
			t.Fatal(clusters[i].Name, err)
		}

		defer func(id string) {
			if err := DeleteCluster(id); err != nil {
				t.Error(id, err)
			}
		}(clusters[i].ID)
	}

	for i := range wrong {
		if err := wrong[i].Insert(); err == nil {
			t.Fatal("Name should not allowed Duplicate", wrong[i].Name)
		}
	}

	cl0, err := GetCluster("cluster3")
	if err != nil {
		t.Fatal(err)
	}
	if cl0.ID != clusters[2].ID {
		t.Fatalf("Unexpected,%s != %s", cl0.ID, clusters[2].ID)
	}
	cl1, err := GetCluster(clusters[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	if cl1.Name != clusters[2].Name {
		t.Fatalf("Unexpected,%s != %s", cl1.ID, clusters[2].ID)
	}

	_, err = GetCluster("cluster5")
	if err == nil {
		t.Fatal(err)
	}

	cl2 := Cluster{ID: clusters[2].ID}
	if err := cl2.UpdateStatus(false); err != nil {
		t.Fatal(err)
	}
	cl3, err := GetCluster(cl2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cl2.Enabled != cl3.Enabled {
		t.Fatalf("Unexpected,%t != %t", cl2.Enabled, cl3.Enabled)
	}

	for i := range wrong {
		if err := DeleteCluster(wrong[i].ID); err != nil {
			t.Error(wrong[i].Name, err)
		}
	}
}

func NewNode(name, clusterID, addr, eng string, num, status int, t1, t2 time.Time) Node {
	return Node{
		ID:           utils.Generate64UUID(),
		Name:         name,
		ClusterID:    clusterID,
		Addr:         addr,
		EngineID:     eng,
		MaxContainer: num,
		Status:       status,
		RegisterAt:   t1,
		DeregisterAt: t2,
	}
}

func (n Node) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	// insert into database
	_, err = db.NamedExec(insertNodeQuery, &n)

	return err
}

func ListNode(status int) ([]Node, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	nodes := make([]Node, 0, 50)
	err = db.Select(&nodes, "SELECT * FROM tb_node WHERE status=?", status)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func TestNode(t *testing.T) {
	list := []Node{
		NewNode("node1", "cluster1", "192.168.2.10:8888", "", 100, 1000, time.Now(), time.Time{}),
		NewNode("node2", "cluster2", "192.168.2.144:8888", "ahfafja", 1000, 1000, time.Time{}, time.Now()),
		NewNode("node3", "cluster1", "192.168.2.134:8888", "foaufafajf", 100, 0, time.Time{}, time.Time{}),
		NewNode("node4", "cluster2", "192.168.2.13:8888", "", 0, 1000, time.Now(), time.Now()),
		NewNode("node5", "cluster3", "192.168.2.10:8888", "xxxxxxxx", 100, 100100, time.Now(), time.Time{}),
	}

	for i := range list {
		err := list[i].Insert()
		if err != nil {
			if list[i].Name == "node5" {
				t.Log("Expected:", err)
			} else {
				t.Fatal(list[i].Name, err)
			}
		}

		defer func(id string) {
			if err := DeleteNode(id); err != nil {
				t.Error(id, err)
			}
		}(list[i].ID)
	}

	node1, err := GetNode(list[3].ID)
	if err != nil {
		t.Fatal(err)
	}
	node2, err := GetNode(list[3].Name)
	if err != nil {
		t.Fatal(err)
	}
	if node1 != node2 {
		t.Fatal("Unexpected")
	}

	node1, err = GetNode("node0000")
	if err == nil {
		t.Fatal(err)
	}

	n := Node{ID: list[3].ID}
	err = n.UpdateStatus(9999)
	if err != nil {
		t.Fatal(err)
	}
	node3, err := GetNode(list[3].ID)
	if err != nil {
		t.Fatal(err)
	}

	if n.Status != node3.Status {
		t.Fatalf("Unexpect,%d != %d", n.Status, node3.Status)
	}

	nodes1, err := ListNode(1000)
	if err != nil {
		t.Fatal(err)
	}
	for i := range nodes1 {
		if nodes1[i].ID == "" {
			t.Error("nil value")
		}
	}
	if len(nodes1) != 2 {
		t.Fatal("Unexpect")
	}

	nodes2, err := ListNodeByCluster("cluster2")
	if err != nil {
		t.Fatal(err)
	}
	for i := range nodes2 {
		if nodes1[i].ID == "" {
			t.Error("nil value")
		}
	}
	if len(nodes2) != 2 {
		t.Fatal("Unexpect")
	}

	nodes, err := GetAllNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != len(list)-1 {
		t.Error("Unexpected,%d!=%d", len(nodes), len(list)-1)
	}

	for i := range list {
		IDOrName := list[i].ID
		if i%2 == 0 {
			IDOrName = list[i].Name
		}
		if err := DeleteNode(IDOrName); err != nil {
			t.Error(IDOrName, err)
		}
	}

	t0 := NewTask("qwertyu", "qwertyuiopsdfghjklbn", "tyuighjfdghjkl", nil, 888)
	t1 := NewTask("qweafjlafjrtyu", "qwertyuiajlfakfaopsdfghjklbn", "tyuifajflaghjfdghjkl", nil, 888)
	t2 := NewTask("qwertyu", "qwertyuiopsdfghjklbn", "tyuighjfdghjkl", []string{"jlafjlakf", "jaljflajflajf"}, 888)
	t3 := NewTask("qweafjlahjklkjhfjrtyu", "qwertfajlfjafyuiajlfakfaopsdfghjklbn", "", nil, 888)
	tasks := []*Task{&t0, &t1, &t2, &t3}

	defer func() {
		for i := range tasks {
			if err := DeleteTask(tasks[i].ID); err != nil {
				t.Error(tasks[i].ID, err)
			}
		}
	}()

	list1 := make([]*Node, len(list))
	for i := range list {
		list1[i] = &list[i]
	}

	err = TxInsertMultiNodeAndTask(list1[0:4], tasks[0:])
	if err != nil {
		t.Fatal(err)
	}

	err = TxUpdateNodeStatus(&list[0], tasks[1], 9977, 999, "foafghjk")
	if err != nil {
		t.Fatal(err)
	}

	err = TxUpdateNodeStatus(&list[1], tasks[2], 0, 999, "")
	if err != nil {
		t.Fatal(err)
	}

	err = TxUpdateNodeRegister(&list[1], tasks[2], 0, 999, "", "")
	if err != nil {
		t.Fatal(err)
	}
}

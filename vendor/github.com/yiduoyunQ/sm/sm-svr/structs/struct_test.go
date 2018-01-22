// print out json string for java unit test
package structs

import (
	"encoding/json"
	"os"
	"testing"
)

func TestStruct(t *testing.T) {
	Print(UserFull(), "User_Full.json")
	Print(UserMin(), "User_Min.json")
	Print(AuthInfoFull(), "AuthInfo_Full.json")
	Print(AuthInfoMin(), "AuthInfo_Min.json")
	Print(DatabaseAuthFull(), "DatabaseAuth_Full.json")
	Print(DatabaseAuthMin(), "DatabaseAuth_Min.json")
	Print(DatabaseInfoFull(), "DatabaseInfo_Full.json")
	Print(DatabaseInfoMin(), "DatabaseInfo_Min.json")
	Print(ProxyInfoFull(), "ProxyInfo_Full.json")
	Print(ProxyInfoMin(), "ProxyInfo_Min.json")
	Print(ProxyModeInfoFull(), "ProxyModeInfo_Full.json")
	Print(ProxyModeInfoMin(), "ProxyModeInfo_Min.json")
	Print(TopologyFull(), "Topology_Full.json")
	Print(TopologyMin(), "Topology_Min.json")
	Print(MgmPostFull(), "MgmPost_Full.json")
	Print(MgmPostMin(), "MgmPost_Min.json")
}

func Print(v interface{}, fileName string) {
	b, _ := json.Marshal(v)
	f, _ := os.Create(fileName)
	f.Write(b)
	f.Close()
}

func UserFull() User {
	dbPrivilegesMap := make(map[string][]string)
	dbPrivilegesMap["key1"] = []string{"val11", "val12"}
	dbPrivilegesMap["key2"] = []string{"val21", "val22"}
	user := User{
		Id:              "Id",
		UserName:        "UserName",
		Password:        "Password",
		DbPrivilegesMap: dbPrivilegesMap,
		WhiteList:       []string{"WhiteList1", "WhiteList2"},
		BlackList:       []string{"BlackList1", "BlackList2"},
		ReadOnly:        true,
		RwSplit:         true,
		Shard:           true,
	}
	return user
}

func UserMin() User {
	user := User{}
	return user
}

func AuthInfoFull() AuthInfo {
	proxyModeInfo := ProxyModeInfoFull()
	authInfo := AuthInfo{
		Password:  "password",
		Max:       9,
		Min:       1,
		DbpmName:  "dbpm_name",
		WhiteList: []string{"white_list1", "white_list2"},
		BlackList: []string{"black_list1", "black_list2"},
		Mode:      proxyModeInfo,
	}
	return authInfo
}

func AuthInfoMin() AuthInfo {
	authInfo := AuthInfo{}
	return authInfo
}

func DatabaseAuthFull() DatabaseAuth {
	databaseAuth := DatabaseAuth{}

	databaseUsers := make(map[string]*DatabaseAuthInfo)
	authInfo1 := AuthInfoFull()
	databaseUsers["key1"] = &DatabaseAuthInfo{AuthInfo: authInfo1}
	authInfo2 := AuthInfoFull()
	databaseUsers["key2"] = &DatabaseAuthInfo{AuthInfo: authInfo2}
	databaseAuth.DatabaseUsers = databaseUsers

	proxyDatabaseUserMap := make(map[string]string)
	proxyDatabaseUserMap["key1"] = "value1"
	proxyDatabaseUserMap["key2"] = "value2"
	databaseAuth.ProxyDatabaseUserMap = proxyDatabaseUserMap

	return databaseAuth
}

func DatabaseAuthMin() DatabaseAuth {
	databaseAuth := DatabaseAuth{}
	return databaseAuth
}

func DatabaseInfoFull() DatabaseInfo {
	databaseAuth := DatabaseAuthFull()
	databaseInfo := DatabaseInfo{
		Ip:           "12.34.56.78",
		Port:         10001,
		Status:       "good",
		Type:         "TypeA",
		DatabaseAuth: &databaseAuth,
	}

	return databaseInfo
}

func DatabaseInfoMin() DatabaseInfo {
	databaseInfo := DatabaseInfo{}
	return databaseInfo
}

func ProxyInfoFull() ProxyInfo {
	proxyInfo := ProxyInfo{
		Id:            "id1",
		Name:          "name1",
		Ip:            "12.34.56.78",
		Port:          ":8080",
		ClientAddress: "1.1.1.1",
		ProxyAddress:  "2.2.2.2",
		StartupTime:   "2017-01-01",
		Status:        0,
		ActiveTime:    31560,
	}

	return proxyInfo
}

func ProxyInfoMin() ProxyInfo {
	proxyInfo := ProxyInfo{}
	return proxyInfo
}

func ProxyModeInfoFull() ProxyModeInfo {
	proxyModeInfo := ProxyModeInfo{
		IsShard:   true,
		IsRwSplit: true,
		IsOnly:    true,
		Datanode:  "datanode",
	}

	return proxyModeInfo
}

func ProxyModeInfoMin() ProxyModeInfo {
	proxyModeInfo := ProxyModeInfo{}
	return proxyModeInfo
}

func TopologyFull() Topology {
	proxyModeInfo := ProxyModeInfoFull()

	proxyUsers := make(map[string]*ProxyAuthInfo)
	authInfo1 := AuthInfoFull()
	proxyUsers["key1"] = &ProxyAuthInfo{AuthInfo: authInfo1}
	authInfo2 := AuthInfoFull()
	proxyUsers["key2"] = &ProxyAuthInfo{AuthInfo: authInfo2}

	proxyGroups := make(map[string]*ProxyInfo)
	proxyInfo1 := ProxyInfoFull()
	proxyGroups["key1"] = &proxyInfo1
	proxyInfo2 := ProxyInfoFull()
	proxyGroups["key2"] = &proxyInfo2

	databaseAuth := DatabaseAuthFull()

	dataNodeGroup := make(map[string]map[string]*DatabaseInfo)
	dataNodes := make(map[string]*DatabaseInfo)
	databaseInfo1 := DatabaseInfoFull()
	dataNodes["key1"] = &databaseInfo1
	databaseInfo2 := DatabaseInfoFull()
	dataNodes["key2"] = &databaseInfo2
	dataNodeGroup["default"] = dataNodes

	dataNodeGroupNormalCount := make(map[string]int)
	dataNodeGroupNormalCount["default"] = 2

	topology := Topology{
		Version:                  "1",
		ProxyMode:                &proxyModeInfo,
		ProxyUsers:               proxyUsers,
		ProxyGroups:              proxyGroups,
		DatabaseAuth:             &databaseAuth,
		DataNodeGroup:            dataNodeGroup,
		DataNodeGroupNormalCount: dataNodeGroupNormalCount,
	}

	return topology
}

func TopologyMin() Topology {
	topology := Topology{}
	return topology
}

func MgmPostFull() MgmPost {
	proxyGroups := make(map[string]*ProxyInfo)
	proxyInfo1 := ProxyInfoFull()
	proxyGroups["key1"] = &proxyInfo1
	proxyInfo2 := ProxyInfoFull()
	proxyGroups["key2"] = &proxyInfo2

	users := []User{UserFull(), UserFull()}

	dataNodeMap := make(map[string]map[string]DatabaseInfo)
	dataNodes1 := make(map[string]DatabaseInfo)
	databaseInfo11 := DatabaseInfoFull()
	dataNodes1["key1"] = databaseInfo11
	databaseInfo12 := DatabaseInfoFull()
	dataNodes1["key2"] = databaseInfo12
	dataNodeMap["key1"] = dataNodes1
	dataNodes2 := make(map[string]DatabaseInfo)
	databaseInfo21 := DatabaseInfoFull()
	dataNodes2["key1"] = databaseInfo21
	databaseInfo22 := DatabaseInfoFull()
	dataNodes2["key2"] = databaseInfo22
	dataNodeMap["key2"] = dataNodes1

	mgmPost := MgmPost{
		DbaasType:           "dbaas-type",
		DbRootUser:          "db-root-user",
		DbRootPassword:      "db-root-password",
		DbReplicateUser:     "db-replicate-user",
		DbReplicatePassword: "db-replicate-password",
		SwarmApiVersion:     "swarm-api-version",
		ProxyGroups:         proxyGroups,
		Users:               users,
		DataNode:            dataNodeMap,
	}

	return mgmPost
}

func MgmPostMin() MgmPost {
	mgmPost := MgmPost{}
	return mgmPost
}

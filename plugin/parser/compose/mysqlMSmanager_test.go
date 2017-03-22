package compose

//import (
//	"testing"

//	"github.com/stretchr/testify/assert"
//)

//func getTestMysqls() []Mysql {
//	return []Mysql{
//		Mysql{
//			Ip:   "",
//			Port: 123,

//			Instance: "",

//			ReplicateUser: "",
//			Replicatepwd:  "",

//			Rootuser: "",
//			RootPwd:  "",
//		},
//		Mysql{
//			Ip:   "",
//			Port: 123,

//			Instance: "",

//			ReplicateUser: "",
//			Replicatepwd:  "",

//			Rootuser: "",
//			RootPwd:  "",
//		},

//		Mysql{
//			Ip:   "",
//			Port: 123,

//			Instance: "",

//			ReplicateUser: "",
//			Replicatepwd:  "",

//			Rootuser: "",
//			RootPwd:  "",
//		},
//	}
//}

//func TestMysqlMS(t *testing.T) {
//	mysqls := getTestMysqls()
//	composer, err := newMysqlComposer(mysqls, "127.0.0.1", 1234)
//	assert.Nil(err)

//	assert.Nil(composer.ComposeCluster())
//}

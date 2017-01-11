package resource

import (
	"testing"
)

func TestParsePushImageOutput(t *testing.T) {
	input := `
PPDBAAS01:~ # docker images
REPOSITORY                              TAG                 IMAGE ID            CREATED             SIZE
registry.dbaas.me:8080/upsql-5.6.19     latest              1fb8ad138ea1        6 days ago          1.504 GB
couchbase                               4.1.0               390b25dbfd13        5 weeks ago         375.9 MB
registry.dbaas.me:8080/suse/sles12sp1   latest              86342c43433b        10 weeks ago        98.79 MB
suse/sles12sp1                          1.0.3               86342c43433b        10 weeks ago        98.79 MB
suse/sles12sp1                          latest              86342c43433b        10 weeks ago        98.79 MB
suse/sles12                             1.1.0               757f51a5a71b        3 months ago        100.2 MB
suse/sles12                             latest              757f51a5a71b        3 months ago        100.2 MB
suse/sles11sp4                          1.1.0               df5431a58fb0        3 months ago        77.51 MB
suse/sles11sp4                          latest              df5431a58fb0        3 months ago        77.51 MB
PPDBAAS01:~ # docker tag couchbase:4.1.0 registry.dbaas.me:8080/couchbase:4.1.0
PPDBAAS01:~ # docker ps -a
CONTAINER ID        IMAGE                                 COMMAND             CREATED             STATUS                  PORTS               NAMES
893527cb6eb1        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         3 days ago          Up 3 days                                   My0a0WfE_XX_C8ITI
41f3cdcd2105        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         3 days ago          Up 3 days                                   YgD6AhBF_XX_eLocv
488c1b949788        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         3 days ago          Up 3 days                                   BrFLwaVH_XX_fyN00
6ceb53782cad        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         3 days ago          Up 3 days                                   9NBc6lq1_XX_YMTwP
a26227b9bcac        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         5 days ago          Up 5 days                                   CesnXIrA_XX_BNh97
25cac21e577a        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         5 days ago          Up 5 days                                   lI2V23ze_XX_SB6wP
f7acf65dab55        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         5 days ago          Exited (0) 3 days ago                       8FZSPxES_XX_iEBNZ
e6a31818ee97        registry.dbaas.me:8080/upsql-5.6.19   "/bin/bash"         5 days ago          Up 5 days                                   fINsogZK_XX_Zf9Az
PPDBAAS01:~ # docker images
REPOSITORY                              TAG                 IMAGE ID            CREATED             SIZE
registry.dbaas.me:8080/upsql-5.6.19     latest              1fb8ad138ea1        6 days ago          1.504 GB
couchbase                               4.1.0               390b25dbfd13        5 weeks ago         375.9 MB
registry.dbaas.me:8080/couchbase        4.1.0               390b25dbfd13        5 weeks ago         375.9 MB
registry.dbaas.me:8080/suse/sles12sp1   latest              86342c43433b        10 weeks ago        98.79 MB
suse/sles12sp1                          1.0.3               86342c43433b        10 weeks ago        98.79 MB
suse/sles12sp1                          latest              86342c43433b        10 weeks ago        98.79 MB
suse/sles12                             1.1.0               757f51a5a71b        3 months ago        100.2 MB
suse/sles12                             latest              757f51a5a71b        3 months ago        100.2 MB
suse/sles11sp4                          1.1.0               df5431a58fb0        3 months ago        77.51 MB
suse/sles11sp4                          latest              df5431a58fb0        3 months ago        77.51 MB
PPDBAAS01:~ # docker push registry.dbaas.me:8080/couchbase:4.1.0
The push refers to a repository [registry.dbaas.me:8080/couchbase]
5f70bf18a086: Mounted from upsql-5.6.19 
89fe369a716d: Pushed 
3cc4443fd5e8: Pushed 
2f703c45973e: Pushed 
28164843080c: Pushed 
b5e3e117a183: Pushed 
2fd887dff952: Pushed 
87eddba308fa: Pushed 
cfdcf31598b4: Pushed 
4.1.0: digest: sha256:a2e81da648c3fc7f4ac7970c41515b0bc5e913f3028e8e954ddba738765ace36 size: 3410
`
	id, size, err := parsePushImageOutput([]byte(input))
	if err != nil {
		t.Error(err)
	}

	if size != 3410 ||
		id != "sha256:a2e81da648c3fc7f4ac7970c41515b0bc5e913f3028e8e954ddba738765ace36" {
		t.Error("Unexpect ", id, size)
	}
}

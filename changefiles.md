# change files


	garden server:

    // 开发环境：
	1. go get github/upmio/swarm
    2. 用github/upmio/swarm 替换 github/docker/swarm:
	  rm -rf G$OPATH/src/github/docker/swarm
	  ln -s G$OPATH/src/github/upmio/swarm  G$OPATH/src/github/docker/swarm
    3.在$GOPATH/src/github/docker/swarm 目录下开发
	
		+ .travis.yml	L11-14,L19-21
	
	/api:
	
		+++ master.go
		+++ handlers_master.go	
		+++ middleware.go
		+++ errcode.go
		+++ errcode_test.go
		+   primary.go 	L132	
				
		
	/cli:
	
		+++ configruation.go 
		+++ join_seed.go
		+	commands.go		L32-33,L44-60	
		+	flags.go		L162-225
		+	manage.go		L145-174,L188,L194,L209-215,L222-224,L229,L335,L344-372,L441
		
	/cluster:
	
		+	cluster.go 	L110-116
		+	config.go	L293-364
		+-	engine.go	L568-571,L968-978
		+++ upmio.go
		+++ /mesos/addition.go
		+++ /swarm/addition.go
		+++ /swarm/addition_test.go
		+-	/swarm/cluster.go	L403-405 
		
	/garden:all
	
	/plugin:all
	
	/scheduler
		
		+	/filter/filter.go	L38
		+++ /filter/resource.go
		+-	/node/node.go 		L58-70
		+	/strategy/strategy.go L42
		+++	/strategy/group.go 
	
	/docs:
		+++ mgm-api.postman_collection.json
		
		
	/vendor: ---> vendor.json
			

	seed server:
	/cli:
	    +++ join_seed.go 
	    +   commands.go   seedjoin
		+	flags.go      flSeedAddr args
	/seed:all
	
	
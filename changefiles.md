# change files


	garden server:

    //开发环境：
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
		+   primary.go 	L132	
				
		
	/cli:
	
		+	commands.go	L32-33,L44-50
		+++ configruation.go 	
		+	flags.go		L162-207
		+	manage.go	L143-165,L179,L185,L200-206,L213-215,L220,L335-355
		
	/cluster:
	
		+	cluster.go 	L104-111
		+	config.go	L301-372
		+-	engine.go	L943-952,L546-551
		+++ upmio.go
		+++ /mesos/addition.go
		+++ /swarm/addition.go
		+-	/swarm/cluster.go	L403-405 
		
	/garden:all
	
	/plugin:all
	
	/scheduler
		
		+	/filter/filter.go	L38
		+++ /filter/resource.go
		+-	/node/node.go 		L58-70
		+	/strategy/strategy.go L42
		+++	/strategy/group.go 
	
	/vendor: ---> vendor.json
			

	seed server:
	/cli:
	    +++ join_seed.go 
	    +   commands.go   seedjoin
		+	flags.go      flSeedAddr args
	/seed:all
	
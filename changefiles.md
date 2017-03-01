# change files


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
		+-	engine.go	L943-952
		+++ upmio.go
		+++ /mesos/addition.go
		+++ /swarm/addition.go
		
	/garden:all
	
	/plugin:all
	
	/scheduler
		
		+	/filter/filter.go	L38
		+++ /filter/resource.go
		+-	/node/node.go 		L58-70
	
	/vendor: ---> vendor.json
			
		
package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"time"

	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/plugin/client"
	"github.com/docker/swarm/plugin/parser/api"
)

var (
	flHost    = flag.String("addr", "127.0.0.1:8000", "parser server address")
	flImage   = flag.String("image", "image:1.2.3", "image version")
	flConfig  = flag.String("cfgPath", "/tmp/config.json", "template config path,encode by JSON")
	flContext = flag.String("file", "mysql.ini", "image config template context file")
)

func main() {
	flag.Parse()

	iv, err := structs.ParseImage(*flImage)
	if err != nil {
		log.Fatalf("parse image version '%s':%+v", *flImage, err)
	}

	var temp structs.ConfigTemplate

	{
		path, err := utils.GetAbsolutePath(false, *flConfig)
		if err != nil {
			log.Fatalf("template config file not exist:'%s'\n,%+v", *flConfig, err)
		}
		dat, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatalf("open template config file:'%s' error\n,%+v", path, err)
		}

		err = json.Unmarshal(dat, &temp)
		if err != nil {
			log.Fatalf("parse JSON template config file:'%s' error\n,%+v", path, err)
		}
	}

	{
		v := iv.Image()

		if temp.Image != "" {
			t, err := structs.ParseImage(temp.Image)
			if err != nil {
				log.Printf("parse image version '%s':%+v\n", temp.Image, err)
			} else if t.Image() != v {
				log.Fatalf("image:'%s' != '%s' from JSON file", *flImage, temp.Image)
			}
		}
		temp.Image = v
	}

	{
		if *flContext != "" {
			path, err := utils.GetAbsolutePath(false, *flContext)
			if err != nil {
				log.Fatalf("template context file not exist:'%s'\n,%+v", *flContext, err)
			}
			dat, err := ioutil.ReadFile(path)
			if err != nil {
				log.Fatalf("open template context file:'%s' error\n,%+v", path, err)
			}
			temp.Content = string(dat)
			log.Printf("%s context:\n%s", *flContext, dat)
		}
	}

	{
		if temp.Timestamp == 0 {
			temp.Timestamp = time.Now().Unix()
		}

		pc := api.NewPlugin(*flHost, client.NewClient(*flHost, 30*time.Second, nil))

		err := pc.PostImageTemplate(nil, temp)
		if err != nil {
			log.Fatalf("post template file error\n%+v", err)
		}

		log.Printf("image:%s template send", *flImage)
	}
}

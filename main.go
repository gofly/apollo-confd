package main

import (
	"flag"
	"log"

	"gopkg.in/yaml.v2"
)

func main() {
	confPath := flag.String("config", "apollo-confd.yaml", "config file path")
	flag.Parse()
	conf, err := LoadApolloConfdConfig(*confPath)
	if err != nil {
		log.Fatalf("[ERROR] load config with error: %s", err)
	}
	confData, _ := yaml.Marshal(conf)
	log.Printf("[INFO] confd config: %s", string(confData))

	c, err := NewApolloConfd(conf)
	if err != nil {
		log.Fatalf("[ERROR] init apollo confd with error: %s", err)
	}
	changed, err := c.LoadAndWatch()
	if err != nil {
		log.Fatalf("[ERROR] render all config files on startup with error: %s", err)
	}
	log.Printf("[INFO] render all config files, changed: %t", changed)
	select {}
}

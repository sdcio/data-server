package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

var configFile string
var debug bool

func main() {
	pflag.StringVarP(&configFile, "config", "c", "schema-server.yaml", "config file path")
	pflag.BoolVarP(&debug, "debug", "d", false, "enable debug")
	pflag.Parse()
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
START:
	cfg, err := config.New(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read config: %v\n", err)
		os.Exit(1)
	}

	s, err := server.NewServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create a server: %v\n", err)
		os.Exit(1)
	}
	err = s.Serve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to run server: %v", err)
		time.Sleep(time.Second)
		goto START
	}
}

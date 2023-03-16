package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

var configFile string
var debug bool
var trace bool

func main() {
	pflag.StringVarP(&configFile, "config", "c", "schema-server.yaml", "config file path")
	pflag.BoolVarP(&debug, "debug", "d", false, "set log level to DEBUG")
	pflag.BoolVarP(&trace, "trace", "t", false, "set log level to TRACE")
	pflag.Parse()
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetLevel(log.InfoLevel)
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	if trace {
		log.SetLevel(log.TraceLevel)
	}
	var s *server.Server
START:
	if s != nil {
		s.Stop()
	}
	cfg, err := config.New(configFile)
	if err != nil {
		log.Errorf("failed to read config: %v", err)
		os.Exit(1)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Errorf("failed to marshal config: %v", err)
		os.Exit(1)
	}
	log.Infof("read config:\n%s", string(b))
	s, err = server.NewServer(cfg)
	if err != nil {
		log.Errorf("failed to create server: %v", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = s.Serve(ctx)
	if err != nil {
		log.Errorf("failed to run server: %v", err)
		time.Sleep(time.Second)
		goto START
	}
}

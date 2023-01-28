package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/server"
	"github.com/spf13/pflag"
)

var configFile string

func main() {
	pflag.StringVarP(&configFile, "config", "c", "schema-server.yaml", "config file path")
	pflag.Parse()

START:
	cfg, err := config.New(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read config: %v\n", err)
		os.Exit(1)
	}
	// serverConfig := &server.ServerConfig{
	// 	Address: cfg.Address,
	// 	Secure:  false,
	// 	Schemas: make([]*config.SchemaConfig, 0, len(cfg.Schemas)),
	// }
	// for _, sc := range cfg.Schemas {
	// 	scc, err := schema.NewSchema(sc)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	serverConfig.Schemas = append(serverConfig.Schemas, scc)
	// }
	// TODO: Add TLS

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

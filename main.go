// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"net/http"
	_ "net/http/pprof"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/dslog"
	"github.com/sdcio/data-server/pkg/server"
)

var configFile string
var debug bool
var trace bool
var stop bool

var versionFlag bool
var version = "dev"
var commit = ""

func main() {
	pflag.StringVarP(&configFile, "config", "c", "", "config file path")
	pflag.BoolVarP(&debug, "debug", "d", false, "set log level to DEBUG")
	pflag.BoolVarP(&trace, "trace", "t", false, "set log level to TRACE")
	pflag.BoolVarP(&versionFlag, "version", "v", false, "print version")
	pflag.Parse()

	if versionFlag {
		fmt.Println(version + "-" + commit)
		return
	}

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetLevel(log.InfoLevel)
	if debug {
		log.SetLevel(log.DebugLevel)
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	if trace {
		log.SetLevel(log.TraceLevel)
		slog.SetLogLoggerLevel(dslog.TraceLevel)
	}
	log.Infof("data-server %s-%s", version, commit)

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

	ctx, cancel := context.WithCancel(context.Background())
	setupCloseHandler(cancel)
	s, err = server.New(ctx, cfg)
	if err != nil {
		log.Errorf("failed to create server: %v", err)
		os.Exit(1)
	}

	err = s.Serve(ctx)
	if err != nil {
		if stop {
			return
		}
		log.Errorf("failed to run server: %v", err)
		time.Sleep(time.Second)
		goto START
	}
}

func setupCloseHandler(cancelFn context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-c
		fmt.Fprintf(os.Stderr, "\nreceived signal '%s'. terminating...\n", sig.String())
		stop = true
		cancelFn()
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

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

	"net/http"
	_ "net/http/pprof"

	"github.com/go-logr/logr"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/server"
	logf "github.com/sdcio/logger"
	"github.com/spf13/pflag"
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

	slogOpts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: logf.ReplaceTimeAttr,
	}
	if debug {
		slogOpts.Level = slog.Level(logf.VDebug)
	}
	if trace {
		slogOpts.Level = slog.Level(logf.VTrace)
	}

	log := logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, slogOpts))
	logf.SetDefaultLogger(log)
	ctx := logf.IntoContext(context.Background(), log)

	go func() {
		log.Info("pprof server started", "address", "localhost:6060")
		err := http.ListenAndServe("localhost:6060", nil)
		if err != nil {
			log.Error(err, "pprof server failed")
		}
	}()

	log.Info("data-server bootstrap", "version", version, "commit", commit, "log-level", slogOpts.Level.Level().String())

	var s *server.Server
START:
	if s != nil {
		s.Stop()
	}
	cfg, err := config.New(configFile)
	if err != nil {
		log.Error(err, "failed to read config")
		os.Exit(1)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Error(err, "failed to marshal config")
		os.Exit(1)
	}
	log.Info("read config", "config", string(b))

	// add logger to context
	ctx, cancel := context.WithCancel(ctx)
	setupCloseHandler(cancel)
	s, err = server.New(ctx, cfg)
	if err != nil {
		log.Error(err, "failed to create server")
		os.Exit(1)
	}

	err = s.Serve(ctx)
	if err != nil {
		if stop {
			return
		}
		log.Error(err, "failed to run server")
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

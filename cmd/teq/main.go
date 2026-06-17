// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction package.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

// Command teq is the Isopace container daemon — the jPOS Q2 analog as a runnable.
// It boots from a directory of JSON component descriptors, brings up everything
// declared there (switch connectors, and any custom component types registered
// in the binary), hot-(re)deploys as the files change, and runs until
// SIGINT/SIGTERM.
//
//	go run ./cmd/teq -deploy ./examples/teq/deploy
//
// Each *.json file is one component. A switch link, for example:
//
//	{
//	  "name": "isw", "type": "connector", "enabled": true,
//	  "config": {
//	    "addr": "127.0.0.1:8583", "packager": "iso87-a", "framer": "length2",
//	    "sign_on": {"mti": "0800", "nmic": "001"},
//	    "echo":    {"mti": "0800", "nmic": "301", "interval_ms": 30000},
//	    "header":  {"41": "TERM0001"}
//	  }
//	}
//
// Pair it with a switch to talk to, e.g. the simulator:
//
//	go run ./examples/simulator -addr 127.0.0.1:8583   # terminal 1
//	go run ./cmd/teq -deploy ./examples/teq/deploy      # terminal 2
package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/teqpace-services/isopace/runtime"
	"github.com/teqpace-services/isopace/teq"
)

func main() {
	deployDir := flag.String("deploy", "deploy", "directory of JSON component descriptors")
	level := flag.String("log-level", "info", "log level: debug|info|warn|error")
	format := flag.String("log-format", "text", "log format: text|json")
	scan := flag.Duration("scan", 5*time.Second, "deploy-dir rescan interval")
	flag.Parse()

	logFormat := runtime.LogText
	if strings.EqualFold(*format, "json") {
		logFormat = runtime.LogJSON
	}
	logger := runtime.NewLogger(os.Stdout, runtime.LogOptions{
		Level:  runtime.ParseLevel(*level, slog.LevelInfo),
		Format: logFormat,
	})

	q := teq.New(
		teq.WithLogger(logger),
		teq.WithDeployDir(*deployDir),
		teq.WithScanInterval(*scan),
	)

	logger.Info("teq starting", "deploy", *deployDir, "scan", scan.String())
	if err := q.ListenAndServe(); err != nil {
		logger.Error("teq stopped", "err", err)
		os.Exit(1)
	}
	logger.Info("teq stopped")
}

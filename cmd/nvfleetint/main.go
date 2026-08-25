// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/release"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
)

// Stamped in at link time by the Makefile and goreleaser. They are declared
// here because -X can only reach the main package; everything that reads them
// takes them as a release.BuildInfo.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	args := os.Args[1:]
	if err := execute(context.Background(), buildInfo(), args); err != nil {
		cmdutil.Write(os.Stderr, args, err)
		os.Exit(cmdutil.ExitCode(err))
	}
}

func buildInfo() release.BuildInfo {
	return release.BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}
}

package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/JungHoonGhae/coupang-ctl/internal/cli"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	buildInfo, buildInfoAvailable := debug.ReadBuildInfo()
	currentVersion := resolvedVersion(version, buildInfo, buildInfoAvailable)
	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, currentVersion); err != nil {
		cli.WriteCommandError(os.Stderr, os.Args[1:], err)
		os.Exit(1)
	}
}

func resolvedVersion(linked string, info *debug.BuildInfo, ok bool) string {
	if linked != "" && linked != "dev" {
		return linked
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

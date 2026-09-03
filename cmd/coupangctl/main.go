package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/JungHoonGhae/coupang-ctl/internal/cli"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, version); err != nil {
		cli.WriteCommandError(os.Stderr, os.Args[1:], err)
		os.Exit(1)
	}
}

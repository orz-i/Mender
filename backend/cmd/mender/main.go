// Command mender is the local-first Mender CLI entry point.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/orz-i/Mender/backend/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

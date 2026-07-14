package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/SmonSisay/winprovision/internal/executor"
	"github.com/SmonSisay/winprovision/internal/progress"
)

var version = "1.0.0"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	display := progress.NewDisplay(0)
	exitCode := executor.Run(ctx, executor.Options{
		Version: version,
		Confirm: display.Confirm,
	})
	os.Exit(exitCode)
}

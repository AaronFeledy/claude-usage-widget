package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	serverapp "github.com/AaronFeledy/claude-usage-widget/server/app"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(os.Args[1:], os.Environ(), logger); err != nil {
		logger.Error("usage server failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(args []string, env []string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serverapp.Run(ctx, args, env, logger, version, os.Stdin, os.Stdout)
}

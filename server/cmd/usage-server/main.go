package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/api"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/config"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/poller"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/providers/claude"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/providers/codex"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/providers/cursor"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/providers/grok"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/server"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

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
	cfg, err := config.Load(ctx, config.LoadOptions{Args: args, Env: env})
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if err := api.ValidateStartup(cfg.ListenAddr, cfg.AuthToken); err != nil {
		return err
	}
	providerPoller, cursorClient, names, err := buildPoller(cfg)
	if err != nil {
		return err
	}
	handler := api.NewHandler(api.Options{Cache: providerPoller, Cursor: cursorClient, Poller: providerPoller, Logger: logger, AuthToken: cfg.AuthToken, ProviderNames: names})
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	return runServerAndPoller(ctx, appRuntime{server: server.RunOptions{Listener: listener, Handler: handler, Logger: logger}, poller: providerPoller, interval: cfg.PollInterval})
}

type appRuntime struct {
	server   server.RunOptions
	poller   *poller.Poller
	interval time.Duration
}

func buildPoller(cfg config.Config) (*poller.Poller, *cursor.Client, []string, error) {
	allowLocalDiscovery, err := api.IsLoopbackListenAddr(cfg.ListenAddr)
	if err != nil {
		return nil, nil, nil, err
	}
	providerPoller := poller.New(poller.Options{})
	var cursorClient *cursor.Client
	names := []string{}
	for name, providerCfg := range cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}
		provider, err := buildProvider(name, providerCfg, allowLocalDiscovery)
		if err != nil {
			return nil, nil, nil, err
		}
		if c, ok := provider.(*cursor.Client); ok {
			cursorClient = c
		}
		if err := providerPoller.Register(provider, true); err != nil {
			return nil, nil, nil, err
		}
		names = append(names, provider.Name())
	}
	return providerPoller, cursorClient, names, nil
}

func buildProvider(name string, providerCfg config.ProviderConfig, allowLocalDiscovery bool) (usage.Provider, error) {
	switch strings.ToLower(name) {
	case "claude":
		return claude.New(claude.Options{CredentialsPath: providerCfg.CredentialsPath}), nil
	case "codex":
		return codex.New(codex.Options{CredentialsPath: providerCfg.CredentialsPath}), nil
	case "cursor":
		return cursor.NewClient(cursor.Options{AuthPath: providerCfg.CredentialsPath, AllowLocalDiscovery: allowLocalDiscovery}), nil
	case "grok":
		return grok.NewProvider(grok.Options{CredentialsPath: providerCfg.CredentialsPath})
	default:
		return nil, fmt.Errorf("unknown provider %q: %w", name, config.ErrInvalidConfig)
	}
}

func runServerAndPoller(ctx context.Context, runtime appRuntime) error {
	appCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		err := runtime.poller.Run(appCtx, runtime.interval)
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		errCh <- err
	}()
	go func() { errCh <- server.Run(appCtx, runtime.server) }()
	first := <-errCh
	cancel()
	second := <-errCh
	if first != nil {
		return first
	}
	return second
}

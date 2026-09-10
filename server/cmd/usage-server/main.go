package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
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
	return runContext(ctx, args, env, logger, desktopSessionOptions{input: os.Stdin, output: os.Stdout})
}

func runContext(ctx context.Context, args []string, env []string, logger *slog.Logger, desktopOptions desktopSessionOptions) error {
	cfg, err := config.Load(ctx, config.LoadOptions{Args: args, Env: env})
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if cfg.DesktopSession {
		return runDesktopSession(ctx, cfg, logger, desktopOptions)
	}
	if err := api.ValidateStartup(cfg.ListenAddr, cfg.AuthToken); err != nil {
		return err
	}
	providerPoller, cursorClient, grokProvider, names, err := buildPoller(cfg)
	if err != nil {
		return err
	}
	handler := api.NewHandler(api.Options{
		Cache:         providerPoller,
		Cursor:        cursorClient,
		Grok:          grokProvider,
		Poller:        providerPoller,
		Logger:        logger,
		AuthToken:     cfg.AuthToken,
		Version:       version,
		ProviderNames: names,
	})
	listener, err := desktopOptions.withDefaults().listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	return runServerAndPoller(ctx, appRuntime{server: server.RunOptions{Listener: listener, Handler: handler, Logger: logger}, poller: providerPoller, interval: cfg.PollInterval})
}

func runDesktopSession(ctx context.Context, cfg config.Config, logger *slog.Logger, options desktopSessionOptions) error {
	prepared, err := prepareDesktopSession(ctx, cfg.ListenAddr, options)
	if err != nil {
		return err
	}
	defer prepared.listener.Close()
	cfg.AuthToken = prepared.request.Token
	providerPoller, cursorClient, grokProvider, names, err := buildPoller(cfg)
	if err != nil {
		return err
	}
	handler := api.NewHandler(api.Options{
		Cache:         providerPoller,
		Cursor:        cursorClient,
		Grok:          grokProvider,
		Poller:        providerPoller,
		Logger:        logger,
		AuthToken:     cfg.AuthToken,
		Version:       version,
		ProviderNames: names,
	})
	if err := prepared.publishIdentity(); err != nil {
		return err
	}
	return runServerAndPoller(ctx, appRuntime{
		server:   server.RunOptions{Listener: prepared.listener, Handler: handler, Logger: logger},
		poller:   providerPoller,
		interval: cfg.PollInterval,
	})
}

type appRuntime struct {
	server   server.RunOptions
	poller   *poller.Poller
	interval time.Duration
}

func buildPoller(cfg config.Config) (*poller.Poller, *cursor.Client, *grok.Provider, []string, error) {
	allowLocalDiscovery, err := api.IsLoopbackListenAddr(cfg.ListenAddr)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	providerPoller := poller.New(poller.Options{})
	var cursorClient *cursor.Client
	var grokProvider *grok.Provider
	names := []string{}
	for name, providerCfg := range cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}
		provider, err := buildProvider(name, providerCfg, allowLocalDiscovery)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if c, ok := provider.(*cursor.Client); ok {
			cursorClient = c
		}
		if g, ok := provider.(*grok.Provider); ok {
			grokProvider = g
		}
		if err := providerPoller.Register(provider, true); err != nil {
			return nil, nil, nil, nil, err
		}
		names = append(names, provider.Name())
	}
	return providerPoller, cursorClient, grokProvider, names, nil
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

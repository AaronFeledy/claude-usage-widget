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
	"os/user"
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
	"github.com/AaronFeledy/claude-usage-widget/server/internal/sshaccess"
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
	stdioMode, err := sshaccess.IsStdioMode(args)
	if err != nil {
		return err
	}
	if stdioMode {
		if err := sshaccess.ValidatePlatform(); err != nil {
			return err
		}
		home, err := runtimeHome(desktopOptions)
		if err != nil {
			return err
		}
		return sshaccess.ServeStdio(ctx, desktopOptions.input, desktopOptions.output, home)
	}
	cfg, err := config.Load(ctx, config.LoadOptions{Args: args, Env: env})
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if cfg.DesktopSession {
		if cfg.SSHAccess {
			return fmt.Errorf("desktop-session and ssh-access cannot be combined: %w", config.ErrInvalidConfig)
		}
		return runDesktopSession(ctx, cfg, logger, desktopOptions)
	}
	if cfg.SSHAccess {
		if err := sshaccess.ValidatePlatform(); err != nil {
			return err
		}
	}
	if err := api.ValidateStartup(cfg.ListenAddr, cfg.AuthToken); err != nil {
		return err
	}
	var sshListener net.Listener
	if cfg.SSHAccess {
		home, err := runtimeHome(desktopOptions)
		if err != nil {
			return err
		}
		sshListener, err = sshaccess.Listen(home)
		if err != nil {
			return err
		}
		defer sshListener.Close()
	}
	providerPoller, codexClient, cursorClient, grokProvider, names, err := buildPoller(cfg)
	if err != nil {
		return err
	}
	handler := api.NewHandler(api.Options{
		Cache:         providerPoller,
		Cursor:        cursorClient,
		Grok:          grokProvider,
		Codex:         codexClient,
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
	servers := []server.RunOptions{{Listener: listener, Handler: handler, Logger: logger}}
	if sshListener != nil {
		servers = append(servers, server.RunOptions{Listener: sshListener, Handler: sshaccess.InjectAuthorization(cfg.AuthToken, handler), Logger: logger})
	}
	return runServerAndPoller(ctx, appRuntime{servers: servers, poller: providerPoller, interval: cfg.PollInterval})
}

func runDesktopSession(ctx context.Context, cfg config.Config, logger *slog.Logger, options desktopSessionOptions) error {
	ctx, stopParentWatch := desktopParentContext(ctx)
	defer stopParentWatch()
	prepared, err := prepareDesktopSession(ctx, cfg.ListenAddr, options)
	if err != nil {
		return err
	}
	defer prepared.listener.Close()
	cfg.AuthToken = prepared.request.Token
	providerPoller, codexClient, cursorClient, grokProvider, names, err := buildPoller(cfg)
	if err != nil {
		return err
	}
	handler := api.NewHandler(api.Options{
		Cache:         providerPoller,
		Cursor:        cursorClient,
		Grok:          grokProvider,
		Codex:         codexClient,
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
		servers:  []server.RunOptions{{Listener: prepared.listener, Handler: handler, Logger: logger}},
		poller:   providerPoller,
		interval: cfg.PollInterval,
	})
}

type appRuntime struct {
	servers  []server.RunOptions
	poller   *poller.Poller
	interval time.Duration
}

func buildPoller(cfg config.Config) (*poller.Poller, *codex.Client, *cursor.Client, *grok.Provider, []string, error) {
	allowLocalDiscovery, err := api.IsLoopbackListenAddr(cfg.ListenAddr)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	providerPoller := poller.New(poller.Options{})
	var cursorClient *cursor.Client
	var codexClient *codex.Client
	var grokProvider *grok.Provider
	names := []string{}
	for name, providerCfg := range cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}
		provider, err := buildProvider(name, providerCfg, allowLocalDiscovery)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		if c, ok := provider.(*cursor.Client); ok {
			cursorClient = c
		}
		if c, ok := provider.(*codex.Client); ok {
			codexClient = c
		}
		if g, ok := provider.(*grok.Provider); ok {
			grokProvider = g
		}
		if err := providerPoller.Register(provider, true); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		names = append(names, provider.Name())
	}
	return providerPoller, codexClient, cursorClient, grokProvider, names, nil
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
	errCh := make(chan error, len(runtime.servers)+1)
	go func() {
		err := runtime.poller.Run(appCtx, runtime.interval)
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		errCh <- err
	}()
	for _, options := range runtime.servers {
		options := options
		go func() { errCh <- server.Run(appCtx, options) }()
	}
	first := <-errCh
	cancel()
	var shutdownErr error
	for range runtime.servers {
		if err := <-errCh; shutdownErr == nil && err != nil {
			shutdownErr = err
		}
	}
	if first != nil {
		return first
	}
	return shutdownErr
}

func runtimeHome(options desktopSessionOptions) (string, error) {
	if options.homeDir != "" {
		return options.homeDir, nil
	}
	current, err := user.Current()
	if err != nil || current.HomeDir == "" {
		return "", fmt.Errorf("resolve current account home")
	}
	return current.HomeDir, nil
}

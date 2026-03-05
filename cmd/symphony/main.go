package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/bjk/symphony/internal/agent"
	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/logging"
	"github.com/bjk/symphony/internal/orchestrator"
	"github.com/bjk/symphony/internal/tracker"
	"github.com/bjk/symphony/internal/web"
	"github.com/bjk/symphony/internal/workspace"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "symphony: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	// Subcommand routing
	if len(args) > 0 {
		switch args[0] {
		case "init":
			return runInit(args[1:], stdout, stderr)
		case "create-issues":
			return runCreateIssues(args[1:], stdout, stderr)
		}
	}

	fs := flag.NewFlagSet("symphony", flag.ContinueOnError)
	portFlag := fs.Int("port", 0, "HTTP dashboard port (overrides config)")
	versionFlag := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *versionFlag {
		fmt.Printf("symphony %s (built %s)\n", version, buildTime)
		return nil
	}

	workflowPath := "./WORKFLOW.md"
	if fs.NArg() > 0 {
		workflowPath = fs.Arg(0)
	}

	logger := logging.Setup(slog.LevelInfo)

	// Config watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var orch *orchestrator.Orchestrator
	watcher, err := config.NewConfigWatcher(workflowPath, func() {
		logger.Info("workflow config reloaded")
		if orch != nil {
			orch.Events() <- domain.WorkflowReloadEvent{}
		}
	})
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	defer watcher.Stop()

	cfg, _ := watcher.Current()

	// Port override
	port := cfg.Server.Port
	if *portFlag != 0 {
		port = *portFlag
	}

	// Tracker
	ghRunner := &execCommandRunner{}
	tr := tracker.NewGitHubClient(
		cfg.Tracker.Repo,
		cfg.Tracker.ActiveStates,
		cfg.Tracker.TerminalStates,
		ghRunner,
	)

	// Workspace
	exec := &shellExecutor{}
	wsMgr := workspace.NewManager(
		cfg.Workspace.Root,
		cfg.Workspace.RepoURL,
		cfg.Workspace.BaseBranch,
		exec,
	)
	hooks := workspace.NewHookRunner(workspace.HookConfig{
		AfterCreate:  cfg.Hooks.AfterCreate,
		BeforeRun:    cfg.Hooks.BeforeRun,
		AfterRun:     cfg.Hooks.AfterRun,
		BeforeRemove: cfg.Hooks.BeforeRemove,
		TimeoutMs:    cfg.Hooks.TimeoutMs,
	}, exec)

	// Setup bare repo
	if err := wsMgr.Setup(ctx); err != nil {
		return fmt.Errorf("workspace setup: %w", err)
	}

	// Agent runner
	agentRunner := agent.NewRunner(cfg.Claude, &execProcessRunner{}, logger)

	// Orchestrator
	orch = orchestrator.New(orchestrator.Deps{
		Tracker:   tr,
		Workspace: wsMgr,
		Hooks:     hooks,
		Agent:     agentRunner,
		Config:    watcher.Current,
		Logger:    logger,
	})

	// Web server
	webSrv := web.NewServer(orch, logger)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: webSrv.Handler(),
	}

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server
	go func() {
		logger.Info("dashboard starting", "port", port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
		}
	}()

	// Start orchestrator in background
	orchDone := make(chan error, 1)
	go func() {
		orchDone <- orch.Run(ctx)
	}()

	// Wait for signal or orchestrator exit
	select {
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
	case err := <-orchDone:
		if err != nil {
			logger.Error("orchestrator exited with error", "error", err)
		}
	}

	// Graceful shutdown
	cancel()
	httpSrv.Shutdown(context.Background())

	_, _ = stdout, stderr
	return nil
}

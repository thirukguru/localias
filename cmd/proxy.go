// Package cmd — proxy start/stop commands.
// Manages the background reverse proxy daemon process.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/cmd
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/thirukguru/localias/internal/cert"
	"github.com/thirukguru/localias/internal/daemon"
	"github.com/thirukguru/localias/internal/dashboard"
	"github.com/thirukguru/localias/internal/health"
	"github.com/thirukguru/localias/internal/proxy"
	"github.com/thirukguru/localias/internal/traffic"
)

var (
	httpsEnabled bool
	foreground   bool
	certFile     string
	keyFile      string
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Manage the reverse proxy daemon",
}

var proxyStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the reverse proxy daemon",
	RunE:  runProxyStart,
}

var proxyStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the reverse proxy daemon",
	RunE:  runProxyStop,
}

func init() {
	proxyStartCmd.Flags().BoolVar(&httpsEnabled, "https", false, "Enable HTTPS with auto-generated certificates")
	proxyStartCmd.Flags().BoolVar(&foreground, "foreground", false, "Run in foreground (don't daemonize)")
	proxyStartCmd.Flags().StringVar(&certFile, "cert", "", "Path to TLS certificate file")
	proxyStartCmd.Flags().StringVar(&keyFile, "key", "", "Path to TLS key file")

	proxyCmd.AddCommand(proxyStartCmd, proxyStopCmd)
	rootCmd.AddCommand(proxyCmd)
}

func runProxyStart(cmd *cobra.Command, args []string) error {
	dir := GetStateDir()
	port := GetProxyPort()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	// If not foreground, daemonize
	if !foreground {
		d := daemon.NewDaemon(dir, slog.Default())

		// Check if already running
		if d.IsRunning() {
			fmt.Println("✓ Proxy daemon is already running")
			return nil
		}

		daemonArgs := []string{"proxy", "start", "--foreground", "--state-dir", dir, "--port", fmt.Sprintf("%d", port)}
		if httpsEnabled {
			daemonArgs = append(daemonArgs, "--https")
		}
		if certFile != "" {
			daemonArgs = append(daemonArgs, "--cert", certFile)
		}
		if keyFile != "" {
			daemonArgs = append(daemonArgs, "--key", keyFile)
		}

		if err := d.Daemonize(daemonArgs); err != nil {
			return fmt.Errorf("daemonizing: %w", err)
		}

		fmt.Printf("✓ Proxy daemon started on port %d\n", port)
		return nil
	}

	// Foreground mode — actually run the proxy
	return runProxyForeground(dir, port)
}

func runProxyForeground(dir string, port int) error {
	// Setup logging
	logFile, err := os.OpenFile(filepath.Join(dir, "localias.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))

	d := daemon.NewDaemon(dir, logger)
	if err := d.WritePID(); err != nil {
		return fmt.Errorf("writing PID: %w", err)
	}
	if err := d.WritePort(port); err != nil {
		return fmt.Errorf("writing port: %w", err)
	}

	// Auto-generate TLS certs if HTTPS enabled without explicit cert/key
	useHTTPS := IsHTTPSEnabled()
	tlsCert := certFile
	tlsKey := keyFile
	if useHTTPS && tlsCert == "" && tlsKey == "" {
		certsDir := filepath.Join(dir, "certs")
		var err error
		tlsCert, tlsKey, err = cert.EnsureLeafCert(certsDir)
		if err != nil {
			return fmt.Errorf("auto-generating TLS certs: %w", err)
		}
		logger.Info("auto-generated TLS certificates", "cert", tlsCert, "key", tlsKey)
	}

	// Create proxy server
	srv := proxy.NewServer(proxy.ServerConfig{
		Port:     port,
		HTTPS:    useHTTPS,
		TLSCert:  tlsCert,
		TLSKey:   tlsKey,
		StateDir: dir,
		Logger:   logger,
	})

	// Create health checker and traffic logger
	healthChecker := health.NewChecker(logger)
	trafficLogger := traffic.NewLogger(1000)

	// Create and attach dashboard to proxy
	dash := dashboard.New(dashboard.Config{
		Routes:  srv.Routes(),
		Traffic: trafficLogger,
		Health:  healthChecker,
		Logger:  logger,
		Port:    port,
		HTTPS:   useHTTPS,
	})
	srv.SetDashboard(dash.Handler())

	// Create and start RPC server
	rpcServer := daemon.NewRPCServer(d.SocketPath(), logger)
	registerRPCHandlers(rpcServer, srv.Routes(), healthChecker, trafficLogger, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start RPC server
	go rpcServer.Start(ctx)

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		healthChecker.StopAll()
		cancel()
		rpcServer.Stop()
		d.Cleanup()
	}()

	// Start proxy (blocks)
	return srv.Start(ctx)
}

func registerRPCHandlers(rpcServer *daemon.RPCServer, routes *proxy.RouteTable, hc *health.Checker, tl *traffic.Logger, logger *slog.Logger) {
	rpcServer.Handle("register", func(params json.RawMessage) (interface{}, error) {
		var p daemon.RegisterParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		r, err := routes.Register(p.Name, p.Port, p.PID, p.Cmd)
		if err != nil {
			return nil, err
		}
		// Start health checking for the new route
		hc.StartChecking(p.Name, p.Port)
		logger.Info("route registered", "name", p.Name, "port", p.Port)
		return daemon.RegisterResult{URL: r.URL}, nil
	})

	rpcServer.Handle("deregister", func(params json.RawMessage) (interface{}, error) {
		var p daemon.DeregisterParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		routes.Deregister(p.Name)
		hc.StopChecking(p.Name)
		logger.Info("route deregistered", "name", p.Name)
		return struct{}{}, nil
	})

	rpcServer.Handle("alias", func(params json.RawMessage) (interface{}, error) {
		var p daemon.AliasParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		_, err := routes.Alias(p.Name, p.Port, p.Force)
		if err != nil {
			return nil, err
		}
		logger.Info("alias registered", "name", p.Name, "port", p.Port)
		return struct{}{}, nil
	})

	rpcServer.Handle("unalias", func(params json.RawMessage) (interface{}, error) {
		var p daemon.UnaliasParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if err := routes.Unalias(p.Name); err != nil {
			return nil, err
		}
		logger.Info("alias removed", "name", p.Name)
		return struct{}{}, nil
	})

	rpcServer.Handle("list", func(params json.RawMessage) (interface{}, error) {
		list := routes.List()
		result := daemon.ListResult{Routes: make([]daemon.RouteInfo, len(list))}
		for i, r := range list {
			result.Routes[i] = daemon.RouteInfo{
				Name:   r.Name,
				Port:   r.Port,
				URL:    r.URL,
				PID:    r.PID,
				Cmd:    r.Cmd,
				Static: r.Static,
			}
		}
		return result, nil
	})

	rpcServer.Handle("health", func(params json.RawMessage) (interface{}, error) {
		var p daemon.HealthParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		r, ok := routes.Lookup(p.Name)
		if !ok {
			return nil, fmt.Errorf("route %q not found", p.Name)
		}
		status := hc.CheckNow(p.Name, r.Port)
		resultStr := "healthy"
		if !status.Healthy {
			resultStr = "unhealthy"
		}
		return daemon.HealthResult{
			Status:    resultStr,
			Latency:   status.Latency.String(),
			LastCheck: status.LastCheck.Format("15:04:05"),
		}, nil
	})

	rpcServer.Handle("traffic", func(params json.RawMessage) (interface{}, error) {
		var p daemon.TrafficParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		limit := p.Limit
		if limit == 0 {
			limit = 50
		}
		entries := tl.List(limit, p.Route)
		return entries, nil
	})

	rpcServer.Handle("stop", func(params json.RawMessage) (interface{}, error) {
		logger.Info("stop requested via RPC")
		go func() {
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(syscall.SIGTERM)
		}()
		return struct{}{}, nil
	})
}

func runProxyStop(cmd *cobra.Command, args []string) error {
	dir := GetStateDir()
	d := daemon.NewDaemon(dir, slog.Default())

	if !d.IsRunning() {
		fmt.Println("✓ Proxy daemon is not running")
		return nil
	}

	if err := d.Stop(); err != nil {
		return fmt.Errorf("stopping daemon: %w", err)
	}

	fmt.Println("✓ Proxy daemon stopped")
	return nil
}

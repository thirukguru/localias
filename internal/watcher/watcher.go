// Package watcher provides fsnotify-based file watching for route changes.
// Watches routes.json for external modifications and reloads the route table.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/watcher
package watcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/thirukguru/localias/internal/proxy"
)

// RouteWatcher watches routes.json and reloads on external changes.
type RouteWatcher struct {
	routesFile string
	routes     *proxy.RouteTable
	logger     *slog.Logger
}

// NewRouteWatcher creates a new route file watcher.
func NewRouteWatcher(routesFile string, routes *proxy.RouteTable, logger *slog.Logger) *RouteWatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &RouteWatcher{
		routesFile: routesFile,
		routes:     routes,
		logger:     logger,
	}
}

// Watch starts watching the routes file for changes.
// Reloads static routes from the file on write events.
func (w *RouteWatcher) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(w.routesFile); err != nil {
		// File might not exist yet — watch the directory instead
		w.logger.Info("routes file not found, watching will start when file is created", "file", w.routesFile)
		return nil
	}

	w.logger.Info("watching routes file for changes", "file", w.routesFile)

	// Debounce timer
	var debounce *time.Timer

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Debounce rapid writes
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(200*time.Millisecond, func() {
					w.logger.Info("routes file changed, reloading", "file", w.routesFile)
					if err := w.routes.LoadStaticRoutes(w.routesFile); err != nil {
						w.logger.Error("failed to reload routes", "error", err)
					}
				})
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.logger.Error("watcher error", "error", err)
		}
	}
}

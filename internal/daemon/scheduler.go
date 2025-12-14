// Package daemon provides the daemon mode scheduler for omblego.
// It runs periodic sync operations and handles signal-based triggering.
package daemon

import (
	"context"
	"log/slog"
	"time"

	"omblego/internal/output"
	syncpkg "omblego/internal/sync"
)

// Options configures the daemon scheduler
type Options struct {
	SyncOptions syncpkg.Options

	// Scheduling
	Interval time.Duration // Time between syncs

	// Callbacks
	OnSync func(result *syncpkg.Result, err error)

	// Logging
	Logger *slog.Logger
}

// DefaultOptions returns options with sensible defaults
func DefaultOptions() Options {
	return Options{
		SyncOptions: syncpkg.DefaultOptions(),
		Interval:    3 * time.Hour,
		Logger:      slog.Default(),
	}
}

// Scheduler manages periodic sync operations
type Scheduler struct {
	syncer  *syncpkg.Syncer
	opts    Options
	forceCh chan struct{}
	logger  *slog.Logger
}

// New creates a new Scheduler
func New(syncer *syncpkg.Syncer, opts Options) *Scheduler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		syncer:  syncer,
		opts:    opts,
		forceCh: make(chan struct{}, 1),
		logger:  logger,
	}
}

// Run starts the scheduler and blocks until context is cancelled.
// The scheduler will:
// - Run an initial sync immediately
// - Run subsequent syncs at the configured interval
// - Respond to ForceSync() calls to trigger immediate syncs
func (s *Scheduler) Run(ctx context.Context) error {
	s.logger.Info("daemon started",
		"interval", s.opts.Interval,
		"device", s.opts.SyncOptions.DeviceName,
	)

	// Run initial sync
	s.runSync(ctx)

	ticker := time.NewTicker(s.opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("daemon shutting down")
			return ctx.Err()

		case <-ticker.C:
			s.runSync(ctx)

		case <-s.forceCh:
			s.logger.Info("forced sync triggered")
			s.runSync(ctx)
			// Reset the ticker after a forced sync
			ticker.Reset(s.opts.Interval)
		}
	}
}

// ForceSync triggers an immediate sync, bypassing the interval wait.
// This is typically called in response to SIGHUP.
// Non-blocking: if a sync is already in progress, the request is queued.
func (s *Scheduler) ForceSync() {
	select {
	case s.forceCh <- struct{}{}:
	default:
		// Channel full, sync already pending
		s.logger.Debug("force sync already pending")
	}
}

// runSync executes a single sync operation
func (s *Scheduler) runSync(ctx context.Context) {
	s.logger.Debug("starting sync")
	startTime := time.Now()

	// Create timeout context for the sync
	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	result, err := s.syncer.Run(syncCtx, s.opts.SyncOptions)

	duration := time.Since(startTime)
	if err != nil {
		s.logger.Error("sync failed",
			"error", err,
			"duration", duration,
		)
	} else {
		s.logger.Info("sync completed",
			"records", result.RecordCount,
			"duration", duration,
		)
	}

	if s.opts.OnSync != nil {
		s.opts.OnSync(result, err)
	}
}

// WriteResult contains the results of writing to output backends
type WriteResult struct {
	Results []output.WriteResult
	Errors  []error
}

package daemon_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"omblego/internal/daemon"
	syncpkg "omblego/internal/sync"
)

func TestScheduler_ForceSync(t *testing.T) {
	// Create scheduler with short interval
	opts := daemon.Options{
		SyncOptions: syncpkg.DefaultOptions(),
		Interval:    1 * time.Hour, // Long interval so only forced sync triggers
	}

	var syncCount atomic.Int32
	opts.OnSync = func(result *syncpkg.Result, err error) {
		syncCount.Add(1)
	}

	syncer := syncpkg.New(nil, nil)
	scheduler := daemon.New(syncer, opts)

	// Create context that cancels after tests
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run scheduler in background
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	// Wait for initial sync
	time.Sleep(50 * time.Millisecond)
	initialCount := syncCount.Load()
	if initialCount < 1 {
		t.Error("Expected at least 1 sync from initial run")
	}

	// Force a sync
	scheduler.ForceSync()
	time.Sleep(50 * time.Millisecond)

	forcedCount := syncCount.Load()
	if forcedCount <= initialCount {
		t.Errorf("Expected sync count to increase after ForceSync, was %d, now %d", initialCount, forcedCount)
	}

	// Wait for shutdown
	<-done
}

func TestScheduler_GracefulShutdown(t *testing.T) {
	opts := daemon.Options{
		SyncOptions: syncpkg.DefaultOptions(),
		Interval:    1 * time.Hour,
	}

	syncer := syncpkg.New(nil, nil)
	scheduler := daemon.New(syncer, opts)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx)
	}()

	// Give scheduler time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Should shut down quickly
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Scheduler did not shut down in time")
	}
}

func TestScheduler_OnSyncCallback(t *testing.T) {
	opts := daemon.Options{
		SyncOptions: syncpkg.DefaultOptions(),
		Interval:    1 * time.Hour,
	}

	var callbackCalled atomic.Bool
	var receivedErr error

	opts.OnSync = func(result *syncpkg.Result, err error) {
		callbackCalled.Store(true)
		receivedErr = err
	}
	opts.SyncOptions.DeviceName = "TEST-DEVICE"

	syncer := syncpkg.New(nil, nil)
	scheduler := daemon.New(syncer, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	scheduler.Run(ctx)

	if !callbackCalled.Load() {
		t.Error("OnSync callback was not called")
	}

	// Since TEST-DEVICE is not a real device, we expect an error
	if receivedErr == nil {
		t.Error("Expected error for unsupported device")
	}
}

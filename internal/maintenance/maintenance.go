package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/store"
)

const (
	DefaultInterval = time.Hour
	defaultBatch    = 1000
	maxBatches      = 100
)

type Runner struct {
	Store    *store.Store
	Logger   *slog.Logger
	Interval time.Duration
	Now      func() time.Time
}

func New(st *store.Store, logger *slog.Logger) *Runner {
	return &Runner{Store: st, Logger: logger, Interval: DefaultInterval, Now: time.Now}
}

func (r *Runner) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.runLogged(ctx)
		ticker := time.NewTicker(r.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runLogged(ctx)
			}
		}
	}()
	return done
}

func (r *Runner) RunOnce(ctx context.Context) (store.CleanupResult, error) {
	now := r.Now().UTC()
	if err := r.Store.StartOperation(ctx, "cleanup", now); err != nil {
		return store.CleanupResult{}, err
	}
	var total store.CleanupResult
	var runErr error
	for batch := 0; batch < maxBatches; batch++ {
		result, err := r.Store.CleanupBatch(ctx, now, defaultBatch)
		if err != nil {
			runErr = err
			break
		}
		total.Add(result)
		if result.PageviewRecords < defaultBatch && result.VisitorRegistrations < defaultBatch && result.AdministratorSessions < defaultBatch {
			break
		}
		if batch == maxBatches-1 {
			runErr = fmt.Errorf("cleanup stopped after %d bounded batches", maxBatches)
		}
	}
	summary := fmt.Sprintf("pageviews=%d registrations=%d sessions=%d", total.PageviewRecords, total.VisitorRegistrations, total.AdministratorSessions)
	if runErr != nil {
		summary += " error=" + runErr.Error()
	}
	if err := r.Store.FinishOperation(ctx, "cleanup", r.Now().UTC(), runErr == nil, summary); err != nil && runErr == nil {
		runErr = err
	}
	return total, runErr
}

func (r *Runner) runLogged(ctx context.Context) {
	result, err := r.RunOnce(ctx)
	if err != nil {
		r.Logger.Error("maintenance cleanup failed", "error", err)
		return
	}
	r.Logger.Info("maintenance cleanup complete", "pageviews", result.PageviewRecords, "registrations", result.VisitorRegistrations, "sessions", result.AdministratorSessions)
}

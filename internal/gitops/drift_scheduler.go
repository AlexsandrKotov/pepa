package gitops

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/security"
)

// DriftDetectionFunc is a callback that executes drift detection for a schedule.
// It returns the drift result summary and any error.
type DriftDetectionFunc func(ctx context.Context, schedule DriftSchedule) (DriftDetectionOutcome, error)

// DriftAlertFunc is a callback that publishes a drift alert event.
// Called by the scheduler when drift is detected during a scheduled run.
type DriftAlertFunc func(schedule DriftSchedule, outcome DriftDetectionOutcome)

// DriftDetectionOutcome holds the result of a scheduled drift detection run.
type DriftDetectionOutcome struct {
	DriftCount    int
	CriticalCount int
	WarningCount  int
	InfoCount     int
	DriftDetails  map[string]interface{}
}

// DriftScheduler manages cron-based drift detection execution.
type DriftScheduler struct {
	repo      *DriftScheduleRepository
	detect    DriftDetectionFunc
	alertFn   DriftAlertFunc
	stopCh    chan struct{}
	reloadCh  chan struct{}
	stopOnce  sync.Once
}

// NewDriftScheduler creates a new DriftScheduler.
func NewDriftScheduler(repo *DriftScheduleRepository, detect DriftDetectionFunc) *DriftScheduler {
	return &DriftScheduler{
		repo:     repo,
		detect:   detect,
		stopCh:   make(chan struct{}),
		reloadCh: make(chan struct{}, 1),
	}
}

// SetAlertFunc sets the alert callback for publishing drift events.
func (s *DriftScheduler) SetAlertFunc(fn DriftAlertFunc) {
	s.alertFn = fn
}

// Reload signals the scheduler to recalculate its next wake-up time.
func (s *DriftScheduler) Reload() {
	select {
	case s.reloadCh <- struct{}{}:
	default:
	}
}

// Start begins the scheduler loop.
func (s *DriftScheduler) Start(ctx context.Context) {
	slog.Info("drift detection scheduler started")

	for {
		waitDuration := s.calculateNextWait(ctx)

		slog.Debug("drift scheduler sleeping", "duration", waitDuration)
		timer := time.NewTimer(waitDuration)

		select {
		case <-ctx.Done():
			timer.Stop()
			slog.Info("drift scheduler stopping (context cancelled)")
			return
		case <-s.stopCh:
			timer.Stop()
			slog.Info("drift scheduler stopped")
			return
		case <-s.reloadCh:
			timer.Stop()
			slog.Debug("drift scheduler reload signal received")
			continue
		case <-timer.C:
			s.processDueSchedules(ctx)
		}
	}
}

// Stop stops the scheduler. Safe to call multiple times.
func (s *DriftScheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// calculateNextWait returns the duration to sleep before the next check.
func (s *DriftScheduler) calculateNextWait(ctx context.Context) time.Duration {
	schedules, err := s.repo.ListAllSchedules(ctx)
	if err != nil {
		return 5 * time.Minute
	}

	now := time.Now()
	var earliest *time.Time

	for i := range schedules {
		sched := &schedules[i]
		if !sched.Enabled || sched.NextRunAt == nil {
			continue
		}
		if earliest == nil || sched.NextRunAt.Before(*earliest) {
			earliest = sched.NextRunAt
		}
	}

	if earliest == nil {
		return 5 * time.Minute
	}

	wait := earliest.Sub(now)
	if wait < 0 {
		return 0
	}
	if wait > 1*time.Hour {
		wait = 1 * time.Hour
	}
	return wait
}

// processDueSchedules checks for and executes due drift detection schedules.
func (s *DriftScheduler) processDueSchedules(ctx context.Context) {
	schedules, err := s.repo.GetDueSchedules(ctx)
	if err != nil {
		slog.Error("failed to get due drift schedules", "error", err)
		return
	}

	for _, schedule := range schedules {
		slog.Info("executing scheduled drift detection",
			"schedule_id", schedule.ID,
			"repo_id", schedule.RepoID,
			"cluster_id", schedule.ClusterID,
		)

		go func(sched DriftSchedule) {
			// Use a bounded context that respects shutdown but allows completion
			runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			// Run drift detection
			outcome, detectErr := s.detect(runCtx, sched)

			now := time.Now()
			sched.LastRunAt = &now

			if detectErr != nil {
				slog.Error("scheduled drift detection failed",
					"schedule_id", sched.ID,
					"error", detectErr,
				)
				status := "error"
				sched.LastRunStatus = &status
				sched.LastDriftCount = 0
			} else {
				status := "success"
				sched.LastRunStatus = &status
				sched.LastDriftCount = outcome.DriftCount

				// Log the detection result
				logEntry := &DriftDetectionLog{
					TenantID:      sched.TenantID,
					ScheduleID:    &sched.ID,
					RepoID:        sched.RepoID,
					ClusterID:     sched.ClusterID,
					ScopePath:     sched.ScopePath,
					TriggeredBy:   "schedule",
					Status:        "success",
					DriftCount:    outcome.DriftCount,
					CriticalCount: outcome.CriticalCount,
					WarningCount:  outcome.WarningCount,
					InfoCount:     outcome.InfoCount,
					DriftDetails:  outcome.DriftDetails,
					CompletedAt:   &now,
				}
				if createErr := s.repo.CreateLog(runCtx, logEntry); createErr != nil {
					slog.Error("failed to create drift detection log", "error", createErr)
				}

				// Publish alert if drift detected and alerts are enabled
				if sched.AlertOnDrift && outcome.DriftCount > 0 && s.alertFn != nil {
					s.alertFn(sched, outcome)
				}
			}

			// Update schedule with last run and next run
			nextRun := security.NextCronRun(sched.CronExpression)
			sched.NextRunAt = &nextRun
			if updateErr := s.repo.Update(ctx, &sched); updateErr != nil {
				slog.Error("failed to update drift schedule", "schedule_id", sched.ID, "error", updateErr)
			}
		}(schedule)
	}
}

// RunManualDrift executes drift detection for a schedule outside of the cron cycle.
// Used by the API for manual "Run Now" triggers.
func (s *DriftScheduler) RunManualDrift(ctx context.Context, schedule DriftSchedule) (DriftDetectionOutcome, error) {
	outcome, err := s.detect(ctx, schedule)

	now := time.Now()
	schedule.LastRunAt = &now

	if err != nil {
		status := "error"
		schedule.LastRunStatus = &status
	} else {
		status := "success"
		schedule.LastRunStatus = &status
		schedule.LastDriftCount = outcome.DriftCount

		logEntry := &DriftDetectionLog{
			TenantID:      schedule.TenantID,
			ScheduleID:    &schedule.ID,
			RepoID:        schedule.RepoID,
			ClusterID:     schedule.ClusterID,
			ScopePath:     schedule.ScopePath,
			TriggeredBy:   "manual",
			Status:        "success",
			DriftCount:    outcome.DriftCount,
			CriticalCount: outcome.CriticalCount,
			WarningCount:  outcome.WarningCount,
			InfoCount:     outcome.InfoCount,
			DriftDetails:  outcome.DriftDetails,
			CompletedAt:   &now,
		}
		if createErr := s.repo.CreateLog(ctx, logEntry); createErr != nil {
			slog.Error("failed to create manual drift detection log", "error", createErr)
		}
	}

	nextRun := security.NextCronRun(schedule.CronExpression)
	schedule.NextRunAt = &nextRun
	if updateErr := s.repo.Update(ctx, &schedule); updateErr != nil {
		slog.Error("failed to update drift schedule after manual run", "error", updateErr)
	}

	return outcome, err
}

// InitNextRun computes and sets next_run_at for a newly created schedule.
func InitNextRun(cronExpr string) time.Time {
	return security.NextCronRun(cronExpr)
}

// ScheduleRepoID is a helper to get the repo ID from a schedule.
func ScheduleRepoID(s *DriftSchedule) uuid.UUID {
	return s.RepoID
}

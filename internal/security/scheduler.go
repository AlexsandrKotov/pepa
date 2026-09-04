package security

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
)

// Scheduler manages recurring security scans based on cron schedules.
type Scheduler struct {
	repo    *repository.SecurityScanRepository
	scanner *Scanner
	stopCh  chan struct{}
	reloadCh chan struct{} // signals the scheduler to recalculate sleep
}

// NewScheduler creates a new Scheduler.
func NewScheduler(repo *repository.SecurityScanRepository, scanner *Scanner) *Scheduler {
	return &Scheduler{
		repo:     repo,
		scanner:  scanner,
		stopCh:   make(chan struct{}),
		reloadCh: make(chan struct{}, 1),
	}
}

// Reload signals the scheduler to recalculate its next wake-up time.
// Call this after creating/updating/deleting schedules.
func (s *Scheduler) Reload() {
	select {
	case s.reloadCh <- struct{}{}:
	default:
		// Already a pending reload signal
	}
}

// Start begins the scheduler loop.
// Instead of fixed-interval polling, it calculates the exact time until the
// next due schedule and sleeps precisely until then.
func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("security scan scheduler started")

	for {
		waitDuration := s.calculateNextWait(ctx)

		slog.Debug("scheduler sleeping until next due schedule", "duration", waitDuration)
		timer := time.NewTimer(waitDuration)

		select {
		case <-ctx.Done():
			timer.Stop()
			slog.Info("security scan scheduler stopping (context cancelled)")
			return
		case <-s.stopCh:
			timer.Stop()
			slog.Info("security scan scheduler stopped")
			return
		case <-s.reloadCh:
			timer.Stop()
			slog.Debug("scheduler reload signal received, recalculating")
			continue
		case <-timer.C:
			s.processDueSchedules(ctx)
		}
	}
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

// calculateNextWait returns the duration to sleep before the next check.
// It finds the earliest next_run_at among all enabled schedules.
// Falls back to 5 minutes if no schedules exist.
func (s *Scheduler) calculateNextWait(ctx context.Context) time.Duration {
	schedules, err := s.repo.ListAllScanSchedules(ctx)
	if err != nil {
		// On error, fall back to a reasonable polling interval
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
		// No enabled schedules — check again in 5 minutes
		return 5 * time.Minute
	}

	wait := earliest.Sub(now)
	if wait < 0 {
		return 0 // Already due
	}
	// Cap at 1 hour to handle clock changes / schedule updates
	if wait > 1*time.Hour {
		wait = 1 * time.Hour
	}
	return wait
}

// processDueSchedules checks for and executes due schedules.
func (s *Scheduler) processDueSchedules(ctx context.Context) {
	schedules, err := s.repo.GetDueSchedules(ctx)
	if err != nil {
		slog.Error("failed to get due schedules", "error", err)
		return
	}

	for _, schedule := range schedules {
		slog.Info("executing scheduled scan", "schedule_id", schedule.ID, "target_id", schedule.TargetID)

		// Run scan in background with detached context
		go func(sched repository.ScanSchedule) {
			scanCtx := context.WithoutCancel(ctx)
			_, err := s.scanner.RunScan(scanCtx, sched.TargetID, sched.TenantID, "schedule")
			if err != nil {
				slog.Error("scheduled scan failed", "schedule_id", sched.ID, "error", err)
			}

			// Update schedule with last run and next run
			now := time.Now()
			sched.LastRunAt = &now
			nextRun := NextCronRun(sched.CronExpression)
			sched.NextRunAt = &nextRun
			if err := s.repo.UpdateScanSchedule(scanCtx, &sched); err != nil {
				slog.Error("failed to update schedule", "schedule_id", sched.ID, "error", err)
			}
		}(schedule)
	}
}

// NextCronRun calculates the next run time from a cron expression.
// Supports basic cron format: minute hour day month weekday
// Examples: "0 2 * * *" (daily at 2am), "0 */6 * * *" (every 6 hours)
// Searches up to 366 days ahead to support monthly and yearly schedules.
func NextCronRun(cronExpr string) time.Time {
	parts := strings.Fields(cronExpr)
	if len(parts) < 5 {
		// Default to 24 hours from now if invalid
		return time.Now().Add(24 * time.Hour)
	}

	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
	next = next.Add(time.Minute) // Start from next minute

	// Try up to 366 days ahead to support yearly schedules
	maxIterations := 366 * 24 * 60
	for i := 0; i < maxIterations; i++ {
		if matchesCron(parts, next) {
			return next
		}
		next = next.Add(time.Minute)
	}

	// Fallback: 24 hours from now
	return time.Now().Add(24 * time.Hour)
}

// matchesCron checks if a time matches a cron expression.
func matchesCron(parts []string, t time.Time) bool {
	return matchField(parts[0], t.Minute(), 0, 59) &&
		matchField(parts[1], t.Hour(), 0, 23) &&
		matchField(parts[2], t.Day(), 1, 31) &&
		matchField(parts[3], int(t.Month()), 1, 12) &&
		matchField(parts[4], int(t.Weekday()), 0, 6)
}

// matchField checks if a value matches a cron field.
func matchField(field string, value, min, max int) bool {
	if field == "*" {
		return true
	}

	// Handle step values: */5
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil {
			return false
		}
		return (value-min)%step == 0
	}

	// Handle ranges: 1-5
	if strings.Contains(field, "-") {
		rangeParts := strings.Split(field, "-")
		if len(rangeParts) == 2 {
			low, err1 := strconv.Atoi(rangeParts[0])
			high, err2 := strconv.Atoi(rangeParts[1])
			if err1 == nil && err2 == nil {
				return value >= low && value <= high
			}
		}
		return false
	}

	// Handle lists: 1,3,5
	if strings.Contains(field, ",") {
		for _, part := range strings.Split(field, ",") {
			if matchField(strings.TrimSpace(part), value, min, max) {
				return true
			}
		}
		return false
	}

	// Exact match
	fieldVal, err := strconv.Atoi(field)
	if err != nil {
		return false
	}
	return fieldVal == value
}

// ParseTargetID is a helper to parse target IDs from various formats.
func ParseTargetID(idStr string) (uuid.UUID, error) {
	return uuid.Parse(idStr)
}

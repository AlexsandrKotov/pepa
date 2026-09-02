package repository

import (
	"testing"
	"time"
)

func TestCronMatches(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		time   time.Time
		expect bool
	}{
		// Wildcard: every minute
		{"wildcard every minute", "* * * * *", time.Date(2026, 9, 2, 14, 30, 0, 0, time.UTC), true},

		// Exact match
		{"exact match 9:00", "0 9 * * *", time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC), true},
		{"exact mismatch 9:01", "0 9 * * *", time.Date(2026, 9, 2, 9, 1, 0, 0, time.UTC), false},

		// Range: weekdays (Mon=1 through Fri=5)
		{"weekday match Wednesday", "0 9 * * 1-5", time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC), true},  // Wed
		{"weekday nomatch Saturday", "0 9 * * 1-5", time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC), false}, // Sat

		// List: specific hours
		{"list match hour 9", "0 9,12,18 * * *", time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC), true},
		{"list nomatch hour 10", "0 9,12,18 * * *", time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC), false},

		// Step: every 15 minutes
		{"step match :00", "*/15 * * * *", time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC), true},
		{"step match :30", "*/15 * * * *", time.Date(2026, 9, 2, 14, 30, 0, 0, time.UTC), true},
		{"step nomatch :07", "*/15 * * * *", time.Date(2026, 9, 2, 14, 7, 0, 0, time.UTC), false},

		// Combined: weekdays 9-17, on the hour
		{"business hours match", "0 9-17 * * 1-5", time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC), true},  // Wed 14:00
		{"business hours nomatch evening", "0 9-17 * * 1-5", time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC), false}, // Wed 20:00

		// Sunday = 0
		{"sunday match", "0 10 * * 0", time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC), true}, // Sep 6, 2026 is Sunday

		// Invalid expressions
		{"too few fields", "* * *", time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC), false},
		{"too many fields", "* * * * * *", time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC), false},
		{"empty string", "", time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC), false},

		// Range with step
		{"range with step match", "0-30/10 * * * *", time.Date(2026, 9, 2, 14, 20, 0, 0, time.UTC), true},
		{"range with step nomatch", "0-30/10 * * * *", time.Date(2026, 9, 2, 14, 25, 0, 0, time.UTC), false},

		// Specific month
		{"september match", "0 0 1 9 *", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true},
		{"october nomatch", "0 0 1 9 *", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cronMatches(tt.expr, tt.time)
			if got != tt.expect {
				t.Errorf("cronMatches(%q, %v) = %v, want %v", tt.expr, tt.time.Format("Mon 15:04"), got, tt.expect)
			}
		})
	}
}

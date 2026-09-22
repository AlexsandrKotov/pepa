package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pepa/pepa/internal/database"
)

type passwordHealthDB struct {
	t              *testing.T
	used, password bool
	err            error
}

func (db passwordHealthDB) QueryRow(ctx context.Context, _ string, args ...any) pgx.Row {
	db.t.Helper()
	if _, ok := ctx.Deadline(); !ok {
		db.t.Fatal("health query must have a timeout")
	}
	if len(args) != 1 || args[0] != uuid.MustParse(database.SuperAdminUserID) {
		db.t.Fatal("health query must target the super admin")
	}
	return db
}

func (db passwordHealthDB) Scan(dest ...any) error {
	if db.err != nil {
		return db.err
	}
	*dest[0].(*bool) = db.used
	*dest[1].(*bool) = db.password
	return nil
}

func TestCheckAdminPasswordHealth(t *testing.T) {
	for _, tc := range []struct {
		name           string
		used, password bool
		err            error
		want           string
	}{
		{name: "setup pending"},
		{name: "password saved", used: true, password: true},
		{name: "missing password", used: true, want: "super admin password is unavailable"},
		{name: "query failure", err: errors.New("database unavailable"), want: "health check failed"},
		{name: "canceled query", err: context.Canceled, want: "health check failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			checkAdminPasswordHealth(t.Context(), passwordHealthDB{t, tc.used, tc.password, tc.err}, logger)
			output := logs.String()
			if tc.want == "" && output != "" {
				t.Fatalf("unexpected health warning: %s", output)
			}
			if tc.want != "" && !strings.Contains(output, tc.want) {
				t.Fatalf("missing diagnostic %q: %s", tc.want, output)
			}
			if tc.err != nil && (strings.Contains(output, "level=ERROR") || strings.Contains(output, "password is unavailable")) {
				t.Fatal("database failure was misreported as missing credentials")
			}
			if tc.err != nil && !strings.Contains(output, tc.err.Error()) {
				t.Fatal("database failure details were lost")
			}
		})
	}
}

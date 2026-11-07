package db_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/franco-roura/ledger-01/backend/internal/db"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestOpenRejectsMalformedConnectionString(t *testing.T) {
	if pool, err := db.Open(context.Background(), "postgres://%zz", 4); err == nil {
		pool.Close()
		t.Fatal("malformed database URL accepted")
	}
}

func TestMigrationsApplyRollbackAndReapply(t *testing.T) {
	ctx := context.Background()
	admin := testdb.Pool(t)
	name := "migration_" + uuid.New().String()[:8]
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted); err != nil {
			t.Error(err)
		}
	})
	dsn, err := url.Parse(testdb.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	dsn.Path = "/" + name
	pool, err := db.Open(ctx, dsn.String(), 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var database string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil || database != name {
		t.Fatalf("migration database: %s, %v", database, err)
	}
	applied, err := db.MigrateUp(ctx, pool)
	if err != nil || len(applied) == 0 {
		t.Fatalf("initial migration: %v, %v", applied, err)
	}
	status, err := db.MigrationStatus(ctx, pool)
	if err != nil || len(status) != len(applied) {
		t.Fatalf("status: %d, %v; applied %d", len(status), err, len(applied))
	}
	if again, err := db.MigrateUp(ctx, pool); err != nil || len(again) != 0 {
		t.Fatalf("migration is not idempotent: %v, %v", again, err)
	}
	if rolled, err := db.MigrateDown(ctx, pool); err != nil || rolled == nil {
		t.Fatalf("rollback: %v, %v", rolled, err)
	}
	if reapplied, err := db.MigrateUp(ctx, pool); err != nil || len(reapplied) != 1 {
		t.Fatalf("reapply: %v, %v", reapplied, err)
	}
	var currencies int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM currencies").Scan(&currencies); err != nil || currencies == 0 {
		t.Fatalf("seeded currencies lost after rollback: %d, %v", currencies, err)
	}
}

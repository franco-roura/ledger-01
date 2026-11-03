package ledger_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/testdb"
)

func TestConcurrentImportCommitConsumesBatchOnce(t *testing.T) {
	f := newImportFixture(t)
	batch, mapping, rows := f.review("01/10/2026;COMPRA KIOSCO AZUL;-250,00\n")
	if len(rows) != 1 || rows[0].Err != "" {
		t.Fatalf("invalid fixture: %+v", rows)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testdb.Pool(t)
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	// Both requests must reach a database lock before the first may finish. Without
	// batch locking both wait at INSERT; with it, one waits on the batch instead.
	if _, err := blocker.Exec(ctx, "LOCK TABLE transactions IN SHARE MODE"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.CommitImport(ctx, f.a, batch, mapping,
				[]ledger.ImportChoice{{Index: 0, CategoryID: f.expense}}, false)
			errs <- err
		}()
	}
	defer func() { cancel(); wg.Wait() }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			AND (query LIKE '-- name: InsertImportedTransactions%'
			     OR query LIKE '-- name: LockImportBatch%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("both import requests did not reach their lock barriers")
		case <-ticker.C:
		}
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(errs)
	succeeded := 0
	for err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		var inv *ledger.InvalidError
		if !errors.As(err, &inv) || inv.Field != "batch_id" {
			t.Fatalf("expected an already-consumed batch error, got %v", err)
		}
	}
	txs := f.transactions()
	if succeeded != 1 || len(txs) != 1 || txs[0].Amount != 250_00 {
		t.Fatalf("one uploaded row: successes=%d transactions=%+v", succeeded, txs)
	}
}

func TestImportCommitLockIsHouseholdScoped(t *testing.T) {
	f := newImportFixture(t)
	batch, mapping, _ := f.review("01/10/2026;COMPRA KIOSCO AZUL;-250,00\n")
	ctx := context.Background()
	stranger := newActor(t, f.svc)
	choices := []ledger.ImportChoice{{Index: 0, CategoryID: f.expense}}
	var inv *ledger.InvalidError
	if _, err := f.svc.CommitImport(ctx, stranger, batch, mapping, choices, false); !errors.As(err, &inv) || inv.Field != "batch_id" {
		t.Fatalf("another household could access the batch: %v", err)
	}
	if n, err := f.svc.CommitImport(ctx, f.a, batch, mapping, choices, false); err != nil || n != 1 {
		t.Fatalf("owner could not consume the batch: %d, %v", n, err)
	}
}

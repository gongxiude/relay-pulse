//go:build postgres

package storage

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- PG test infrastructure ---

var testSchemaSeq atomic.Int64

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func pgTestDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("PG_TEST_HOST", "localhost"),
		envOr("PG_TEST_PORT", "5432"),
		envOr("PG_TEST_USER", "relaypulse"),
		envOr("PG_TEST_PASSWORD", "##RW42eq31"),
		envOr("PG_TEST_DATABASE", "relaypulse"),
	)
}

// newTestPGStore creates an isolated PostgresStorage backed by a temporary schema.
// The schema is dropped on test cleanup.
func newTestPGStore(t *testing.T) *PostgresStorage {
	t.Helper()
	seq := testSchemaSeq.Add(1)
	schema := fmt.Sprintf("pgtest_%d_%d", time.Now().UnixNano()%1_000_000, seq)
	baseDSN := pgTestDSN()
	ctx := context.Background()

	// Create schema via a temporary pool
	tmp, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("connect PG: %v", err)
	}
	if _, err := tmp.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		tmp.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}
	tmp.Close()

	// Connect with search_path pinned to the test schema
	cfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect PG with schema: %v", err)
	}

	store := &PostgresStorage{pool: pool, ctx: ctx}
	if err := store.Init(); err != nil {
		pool.Close()
		t.Fatalf("Init: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		drop, dropErr := pgxpool.New(context.Background(), baseDSN)
		if dropErr == nil {
			_, _ = drop.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
			drop.Close()
		}
	})

	return store
}

func pgRec(key MonitorKey, ts int64) *ProbeRecord {
	return &ProbeRecord{
		Provider:  key.Provider,
		Service:   key.Service,
		Channel:   key.Channel,
		Model:     key.Model,
		Status:    1,
		SubStatus: SubStatusNone,
		HttpCode:  200,
		Latency:   100,
		Timestamp: ts,
	}
}

func mustSavePG(t *testing.T, store *PostgresStorage, r *ProbeRecord) {
	t.Helper()
	if err := store.SaveRecord(r); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}
}

// --- schema introspection helpers ---

func pgTableExists(t *testing.T, store *PostgresStorage, table string) bool {
	t.Helper()
	var n int
	err := store.pool.QueryRow(store.effectiveCtx(),
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`, table).Scan(&n)
	if err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	return n > 0
}

func pgIndexExists(t *testing.T, store *PostgresStorage, index string) bool {
	t.Helper()
	var n int
	err := store.pool.QueryRow(store.effectiveCtx(),
		`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, index).Scan(&n)
	if err != nil {
		t.Fatalf("check index %s: %v", index, err)
	}
	return n > 0
}

func pgColumnExists(t *testing.T, store *PostgresStorage, table, column string) bool {
	t.Helper()
	var n int
	err := store.pool.QueryRow(store.effectiveCtx(),
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
		table, column).Scan(&n)
	if err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	return n > 0
}

func pgCount(t *testing.T, store *PostgresStorage, query string, args ...any) int {
	t.Helper()
	var n int
	if err := store.pool.QueryRow(store.effectiveCtx(), query, args...).Scan(&n); err != nil {
		t.Fatalf("pgCount: %v", err)
	}
	return n
}

// ===== Init =====

func TestPG_Init_TablesAndIndexes(t *testing.T) {
	store := newTestPGStore(t)

	for _, tbl := range []string{"probe_history", "service_states", "status_events", "channel_states", "newapi_channel_snapshots", "newapi_log_sync_cursors", "newapi_logs", "audit_targets", "diagnostic_runs", "diagnostic_steps", "diagnostic_scores"} {
		if !pgTableExists(t, store, tbl) {
			t.Errorf("expected table %q", tbl)
		}
	}

	for _, idx := range []string{
		"idx_probe_history_pscm_ts_cover",
		"idx_probe_history_timestamp",
		"idx_status_events_psc_id",
		"idx_status_events_unique",
	} {
		if !pgIndexExists(t, store, idx) {
			t.Errorf("expected index %q", idx)
		}
	}
}

func TestPG_Init_Columns(t *testing.T) {
	store := newTestPGStore(t)

	for _, col := range []string{
		"id", "provider", "service", "channel", "model",
		"status", "sub_status", "http_code", "latency", "timestamp",
		"error_detail",
	} {
		if !pgColumnExists(t, store, "probe_history", col) {
			t.Errorf("probe_history missing column %q", col)
		}
	}
}

func TestPG_Init_Idempotent(t *testing.T) {
	store := newTestPGStore(t)
	if err := store.Init(); err != nil {
		t.Fatalf("second Init: %v", err)
	}
}

// ===== SaveRecord + GetLatest =====

func TestPG_SaveAndGetLatest(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "prov-a", Service: "svc-a", Channel: "ch-a", Model: "mdl-a"}

	r := pgRec(key, 1_700_000_000)
	r.Status = 0
	r.SubStatus = SubStatusServerError
	r.HttpCode = 503
	r.Latency = 456
	mustSavePG(t, store, r)

	if r.ID == 0 {
		t.Fatal("expected record ID to be set after SaveRecord")
	}

	got, err := store.GetLatest(key.Provider, key.Service, key.Channel, key.Model)
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if got == nil {
		t.Fatal("GetLatest returned nil")
	}
	if got.ID != r.ID {
		t.Errorf("ID: want %d, got %d", r.ID, got.ID)
	}
	if got.Provider != r.Provider || got.Service != r.Service ||
		got.Channel != r.Channel || got.Model != r.Model {
		t.Errorf("key fields mismatch: %+v", got)
	}
	if got.Status != 0 || got.SubStatus != SubStatusServerError ||
		got.HttpCode != 503 || got.Latency != 456 || got.Timestamp != r.Timestamp {
		t.Errorf("value fields mismatch: %+v", got)
	}
}

func TestPG_GetLatest_ReturnsNewest(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "prov-b", Service: "svc-b", Channel: "ch-b", Model: "mdl-b"}

	mustSavePG(t, store, pgRec(key, 1000))
	mustSavePG(t, store, pgRec(key, 3000))
	mustSavePG(t, store, pgRec(key, 2000))

	got, err := store.GetLatest(key.Provider, key.Service, key.Channel, key.Model)
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if got.Timestamp != 3000 {
		t.Errorf("want timestamp 3000, got %d", got.Timestamp)
	}
}

func TestPG_GetLatest_MissingKey(t *testing.T) {
	store := newTestPGStore(t)

	got, err := store.GetLatest("no", "such", "key", "here")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing key, got %+v", got)
	}
}

// ===== GetHistory =====

func TestPG_GetHistory_TimeRange(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "prov-h", Service: "svc-h", Channel: "ch-h", Model: "mdl-h"}

	for _, ts := range []int64{1000, 2000, 3000, 4000} {
		mustSavePG(t, store, pgRec(key, ts))
	}

	recs, err := store.GetHistory(key.Provider, key.Service, key.Channel, key.Model, time.Unix(2000, 0))
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(recs) != 3 { // 2000, 3000, 4000
		t.Fatalf("want 3 records, got %d", len(recs))
	}
	// Must be ascending order
	for i := 1; i < len(recs); i++ {
		if recs[i].Timestamp <= recs[i-1].Timestamp {
			t.Errorf("not ascending at %d: %d <= %d", i, recs[i].Timestamp, recs[i-1].Timestamp)
		}
	}
}

func TestPG_GetHistory_Empty(t *testing.T) {
	store := newTestPGStore(t)

	recs, err := store.GetHistory("x", "y", "z", "w", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected empty, got %d", len(recs))
	}
}

func TestPG_GetHistory_IsolatesDifferentKeys(t *testing.T) {
	store := newTestPGStore(t)
	keyA := MonitorKey{Provider: "p", Service: "s", Channel: "c1", Model: "m"}
	keyB := MonitorKey{Provider: "p", Service: "s", Channel: "c2", Model: "m"}

	mustSavePG(t, store, pgRec(keyA, 1000))
	mustSavePG(t, store, pgRec(keyB, 2000))

	recs, err := store.GetHistory(keyA.Provider, keyA.Service, keyA.Channel, keyA.Model, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(recs) != 1 || recs[0].Channel != "c1" {
		t.Errorf("expected 1 record for keyA channel=c1, got %d", len(recs))
	}
}

// ===== Batch queries =====

func TestPG_GetLatestBatch(t *testing.T) {
	store := newTestPGStore(t)
	keyA := MonitorKey{Provider: "pa", Service: "sa", Channel: "ca", Model: "ma"}
	keyB := MonitorKey{Provider: "pb", Service: "sb", Channel: "cb", Model: "mb"}

	mustSavePG(t, store, pgRec(keyA, 1000))
	mustSavePG(t, store, pgRec(keyA, 3000))
	mustSavePG(t, store, pgRec(keyB, 1500))

	got, err := store.GetLatestBatch([]MonitorKey{keyA, keyB})
	if err != nil {
		t.Fatalf("GetLatestBatch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(got))
	}
	if got[keyA].Timestamp != 3000 {
		t.Errorf("keyA: want ts 3000, got %d", got[keyA].Timestamp)
	}
	if got[keyB].Timestamp != 1500 {
		t.Errorf("keyB: want ts 1500, got %d", got[keyB].Timestamp)
	}
}

func TestPG_GetLatestBatch_EmptyKeys(t *testing.T) {
	store := newTestPGStore(t)

	got, err := store.GetLatestBatch(nil)
	if err != nil {
		t.Fatalf("GetLatestBatch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestPG_GetLatestBatch_MissingKey(t *testing.T) {
	store := newTestPGStore(t)
	keyA := MonitorKey{Provider: "pa", Service: "sa", Channel: "ca", Model: "ma"}
	keyMissing := MonitorKey{Provider: "no", Service: "such", Channel: "key", Model: "x"}

	mustSavePG(t, store, pgRec(keyA, 1000))

	got, err := store.GetLatestBatch([]MonitorKey{keyA, keyMissing})
	if err != nil {
		t.Fatalf("GetLatestBatch: %v", err)
	}
	if _, ok := got[keyA]; !ok {
		t.Error("expected keyA in result")
	}
	if _, ok := got[keyMissing]; ok {
		t.Error("expected keyMissing to be absent")
	}
}

func TestPG_GetHistoryBatch(t *testing.T) {
	store := newTestPGStore(t)
	keyA := MonitorKey{Provider: "pa", Service: "sa", Channel: "ca", Model: "ma"}
	keyB := MonitorKey{Provider: "pb", Service: "sb", Channel: "cb", Model: "mb"}

	for _, ts := range []int64{1000, 2000, 3000} {
		mustSavePG(t, store, pgRec(keyA, ts))
	}
	for _, ts := range []int64{1500, 2500} {
		mustSavePG(t, store, pgRec(keyB, ts))
	}

	got, err := store.GetHistoryBatch([]MonitorKey{keyA, keyB}, time.Unix(1800, 0))
	if err != nil {
		t.Fatalf("GetHistoryBatch: %v", err)
	}
	if len(got[keyA]) != 2 { // 2000, 3000
		t.Errorf("keyA: want 2, got %d", len(got[keyA]))
	}
	if len(got[keyB]) != 1 { // 2500
		t.Errorf("keyB: want 1, got %d", len(got[keyB]))
	}
	// Ascending order
	if len(got[keyA]) == 2 && got[keyA][0].Timestamp >= got[keyA][1].Timestamp {
		t.Errorf("keyA not ascending: %d >= %d", got[keyA][0].Timestamp, got[keyA][1].Timestamp)
	}
}

func TestPG_GetHistoryBatch_EmptyKeys(t *testing.T) {
	store := newTestPGStore(t)

	got, err := store.GetHistoryBatch(nil, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("GetHistoryBatch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

// ===== MigrateChannelData =====

func TestPG_MigrateChannelData(t *testing.T) {
	store := newTestPGStore(t)
	emptyKey := MonitorKey{Provider: "prov-m", Service: "svc-m", Channel: "", Model: "mdl-m"}
	otherKey := MonitorKey{Provider: "prov-x", Service: "svc-x", Channel: "", Model: "mdl-x"}
	existingKey := MonitorKey{Provider: "prov-m", Service: "svc-m", Channel: "existing", Model: "mdl-e"}

	mustSavePG(t, store, pgRec(emptyKey, 1000))
	mustSavePG(t, store, pgRec(emptyKey, 1100))
	mustSavePG(t, store, pgRec(otherKey, 1200))
	mustSavePG(t, store, pgRec(existingKey, 1300))

	mappings := []ChannelMigrationMapping{
		{Provider: "prov-m", Service: "svc-m", Channel: "beta"},
	}
	if err := store.MigrateChannelData(mappings); err != nil {
		t.Fatalf("MigrateChannelData: %v", err)
	}

	// Migrated records should now be in channel "beta"
	migrated, err := store.GetHistory("prov-m", "svc-m", "beta", "mdl-m", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("GetHistory migrated: %v", err)
	}
	if len(migrated) != 2 {
		t.Errorf("want 2 migrated, got %d", len(migrated))
	}

	// Other provider untouched
	remaining := pgCount(t, store,
		`SELECT COUNT(*) FROM probe_history WHERE provider=$1 AND service=$2 AND channel=''`,
		"prov-x", "svc-x")
	if remaining != 1 {
		t.Errorf("expected 1 untouched for prov-x, got %d", remaining)
	}

	// Existing channel untouched
	kept, err := store.GetLatest("prov-m", "svc-m", "existing", "mdl-e")
	if err != nil {
		t.Fatalf("GetLatest kept: %v", err)
	}
	if kept == nil || kept.Channel != "existing" {
		t.Errorf("existing channel record should be preserved, got %+v", kept)
	}
}

func TestPG_MigrateChannelData_NoEmptyChannels(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "p", Service: "s", Channel: "already-set", Model: "m"}
	mustSavePG(t, store, pgRec(key, 1000))

	err := store.MigrateChannelData([]ChannelMigrationMapping{
		{Provider: "p", Service: "s", Channel: "new-ch"},
	})
	if err != nil {
		t.Fatalf("MigrateChannelData: %v", err)
	}
}

// ===== PurgeOldRecords =====

func TestPG_PurgeOldRecords(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "pp", Service: "sp", Channel: "cp", Model: "mp"}

	mustSavePG(t, store, pgRec(key, 1000))
	mustSavePG(t, store, pgRec(key, 2000))
	mustSavePG(t, store, pgRec(key, 4000))

	deleted, err := store.PurgeOldRecords(context.Background(), time.Unix(3000, 0), 100)
	if err != nil {
		t.Fatalf("PurgeOldRecords: %v", err)
	}
	if deleted != 2 {
		t.Errorf("want 2 deleted, got %d", deleted)
	}

	recs, err := store.GetHistory(key.Provider, key.Service, key.Channel, key.Model, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("GetHistory after purge: %v", err)
	}
	if len(recs) != 1 || recs[0].Timestamp != 4000 {
		t.Errorf("unexpected remaining: %+v", recs)
	}
}

func TestPG_PurgeOldRecords_NothingToPurge(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "pn", Service: "sn", Channel: "cn", Model: "mn"}
	mustSavePG(t, store, pgRec(key, 5000))

	deleted, err := store.PurgeOldRecords(context.Background(), time.Unix(1000, 0), 100)
	if err != nil {
		t.Fatalf("PurgeOldRecords: %v", err)
	}
	if deleted != 0 {
		t.Errorf("want 0, got %d", deleted)
	}
}

func TestPG_PurgeOldRecords_BatchSize(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "pb", Service: "sb", Channel: "cb", Model: "mb"}
	for i := 0; i < 10; i++ {
		mustSavePG(t, store, pgRec(key, int64(1000+i)))
	}

	deleted, err := store.PurgeOldRecords(context.Background(), time.Unix(2000, 0), 3)
	if err != nil {
		t.Fatalf("PurgeOldRecords: %v", err)
	}
	if deleted != 3 {
		t.Errorf("want 3 (batch limited), got %d", deleted)
	}

	deleted2, err := store.PurgeOldRecords(context.Background(), time.Unix(2000, 0), 3)
	if err != nil {
		t.Fatalf("second PurgeOldRecords: %v", err)
	}
	if deleted2 != 3 {
		t.Errorf("want 3, got %d", deleted2)
	}
}

// ===== Concurrent access =====

func TestPG_ConcurrentReadWrite(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "pc", Service: "sc", Channel: "cc", Model: "mc"}

	const (
		writers         = 4
		writesPerWriter = 15
		readers         = 4
		readsPerReader  = 10
	)

	errs := make(chan error, writers*writesPerWriter+readers)
	var wg sync.WaitGroup

	base := int64(1_700_000_000)
	for w := 0; w < writers; w++ {
		for i := 0; i < writesPerWriter; i++ {
			idx := w*writesPerWriter + i
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				if err := store.SaveRecord(pgRec(key, base+int64(idx))); err != nil {
					errs <- err
				}
			}(idx)
		}
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < readsPerReader; i++ {
				if _, err := store.GetLatest(key.Provider, key.Service, key.Channel, key.Model); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent error: %v", err)
	}

	count := pgCount(t, store,
		`SELECT COUNT(*) FROM probe_history WHERE provider=$1 AND service=$2 AND channel=$3 AND model=$4`,
		key.Provider, key.Service, key.Channel, key.Model)
	expected := writers * writesPerWriter
	if count != expected {
		t.Errorf("want %d rows, got %d", expected, count)
	}
}

// ===== ServiceState =====

func TestPG_ServiceState_RoundTrip(t *testing.T) {
	store := newTestPGStore(t)

	// Uninitialized → nil
	state, err := store.GetServiceState("p", "s", "c", "m")
	if err != nil {
		t.Fatalf("GetServiceState: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil for uninitialized, got %+v", state)
	}

	// Insert
	newState := &ServiceState{
		Provider:        "p",
		Service:         "s",
		Channel:         "c",
		Model:           "m",
		StableAvailable: 1,
		StreakCount:     5,
		StreakStatus:    1,
		LastRecordID:    42,
		LastTimestamp:   1234,
	}
	if err := store.UpsertServiceState(newState); err != nil {
		t.Fatalf("UpsertServiceState: %v", err)
	}

	got, err := store.GetServiceState("p", "s", "c", "m")
	if err != nil {
		t.Fatalf("GetServiceState after upsert: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil after upsert")
	}
	if got.StableAvailable != 1 || got.StreakCount != 5 || got.LastRecordID != 42 {
		t.Errorf("state mismatch: %+v", got)
	}

	// Upsert (update existing)
	newState.StreakCount = 10
	if err := store.UpsertServiceState(newState); err != nil {
		t.Fatalf("UpsertServiceState update: %v", err)
	}
	got2, _ := store.GetServiceState("p", "s", "c", "m")
	if got2.StreakCount != 10 {
		t.Errorf("want StreakCount=10, got %d", got2.StreakCount)
	}
}

// ===== ChannelState =====

func TestPG_ChannelState_RoundTrip(t *testing.T) {
	store := newTestPGStore(t)

	// Uninitialized → nil
	state, err := store.GetChannelState("p", "s", "c")
	if err != nil {
		t.Fatalf("GetChannelState: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil, got %+v", state)
	}

	// Insert
	cs := &ChannelState{
		Provider:        "p",
		Service:         "s",
		Channel:         "c",
		StableAvailable: 0,
		DownCount:       3,
		KnownCount:      5,
		LastRecordID:    99,
		LastTimestamp:   5678,
	}
	if err := store.UpsertChannelState(cs); err != nil {
		t.Fatalf("UpsertChannelState: %v", err)
	}

	got, err := store.GetChannelState("p", "s", "c")
	if err != nil {
		t.Fatalf("GetChannelState after upsert: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.DownCount != 3 || got.KnownCount != 5 || got.LastRecordID != 99 {
		t.Errorf("state mismatch: %+v", got)
	}

	// Upsert (update existing)
	cs.DownCount = 1
	if err := store.UpsertChannelState(cs); err != nil {
		t.Fatalf("UpsertChannelState update: %v", err)
	}
	got2, _ := store.GetChannelState("p", "s", "c")
	if got2.DownCount != 1 {
		t.Errorf("want DownCount=1, got %d", got2.DownCount)
	}
}

// ===== GetModelStatesForChannel =====

func TestPG_GetModelStatesForChannel(t *testing.T) {
	store := newTestPGStore(t)

	for _, model := range []string{"gpt-4o", "claude-3", "o1"} {
		if err := store.UpsertServiceState(&ServiceState{
			Provider: "p", Service: "s", Channel: "c", Model: model,
			StableAvailable: 1, StreakCount: 1, StreakStatus: 1,
			LastRecordID: 1, LastTimestamp: 1000,
		}); err != nil {
			t.Fatalf("UpsertServiceState %s: %v", model, err)
		}
	}
	// Different channel — should not appear
	if err := store.UpsertServiceState(&ServiceState{
		Provider: "p", Service: "s", Channel: "other", Model: "x",
		StableAvailable: 1, StreakCount: 1, StreakStatus: 1,
		LastRecordID: 1, LastTimestamp: 1000,
	}); err != nil {
		t.Fatalf("UpsertServiceState other: %v", err)
	}

	states, err := store.GetModelStatesForChannel("p", "s", "c")
	if err != nil {
		t.Fatalf("GetModelStatesForChannel: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("want 3 models, got %d", len(states))
	}
	// Ordered by model ASC
	if states[0].Model != "claude-3" || states[1].Model != "gpt-4o" || states[2].Model != "o1" {
		t.Errorf("unexpected order: %s, %s, %s", states[0].Model, states[1].Model, states[2].Model)
	}
}

// ===== SaveStatusEvent + GetStatusEvents =====

func TestPG_SaveStatusEvent_Idempotent(t *testing.T) {
	store := newTestPGStore(t)

	now := time.Now().Unix()
	evt := &StatusEvent{
		Provider:        "p",
		Service:         "s",
		Channel:         "c",
		Model:           "m",
		EventType:       EventTypeDown,
		TriggerRecordID: 100,
		ObservedAt:      now,
		CreatedAt:       now,
	}
	if err := store.SaveStatusEvent(evt); err != nil {
		t.Fatalf("SaveStatusEvent: %v", err)
	}

	// Duplicate should not error (ON CONFLICT DO NOTHING)
	evt2 := &StatusEvent{
		Provider:        "p",
		Service:         "s",
		Channel:         "c",
		Model:           "m",
		EventType:       EventTypeDown,
		TriggerRecordID: 100,
		ObservedAt:      now,
		CreatedAt:       now,
	}
	if err := store.SaveStatusEvent(evt2); err != nil {
		t.Fatalf("SaveStatusEvent duplicate: %v", err)
	}

	events, err := store.GetStatusEvents(0, 100, nil)
	if err != nil {
		t.Fatalf("GetStatusEvents: %v", err)
	}
	count := 0
	for _, e := range events {
		if e.Provider == "p" && e.TriggerRecordID == 100 {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}
}

func TestPG_GetStatusEvents_Filters(t *testing.T) {
	store := newTestPGStore(t)

	now := time.Now().Unix()
	evts := []*StatusEvent{
		{Provider: "openai", Service: "api", Channel: "v1", EventType: EventTypeDown, TriggerRecordID: 1, ObservedAt: now, CreatedAt: now},
		{Provider: "openai", Service: "api", Channel: "v1", EventType: EventTypeUp, TriggerRecordID: 2, ObservedAt: now + 1, CreatedAt: now + 1},
		{Provider: "anthropic", Service: "messages", Channel: "v1", EventType: EventTypeDown, TriggerRecordID: 3, ObservedAt: now + 2, CreatedAt: now + 2},
	}
	for i, e := range evts {
		if err := store.SaveStatusEvent(e); err != nil {
			t.Fatalf("save event %d: %v", i, err)
		}
	}

	// Filter by provider
	got, err := store.GetStatusEvents(0, 100, &EventFilters{Provider: "openai"})
	if err != nil {
		t.Fatalf("GetStatusEvents provider filter: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("filter provider=openai: want 2, got %d", len(got))
	}

	// Filter by type
	got2, err := store.GetStatusEvents(0, 100, &EventFilters{Types: []EventType{EventTypeDown}})
	if err != nil {
		t.Fatalf("GetStatusEvents type filter: %v", err)
	}
	if len(got2) != 2 {
		t.Errorf("filter type=DOWN: want 2, got %d", len(got2))
	}

	// Cursor pagination: skip first event
	all, _ := store.GetStatusEvents(0, 100, nil)
	if len(all) >= 1 {
		got3, err := store.GetStatusEvents(all[0].ID, 100, nil)
		if err != nil {
			t.Fatalf("GetStatusEvents cursor: %v", err)
		}
		if len(got3) != len(all)-1 {
			t.Errorf("cursor pagination: want %d, got %d", len(all)-1, len(got3))
		}
	}
}

func TestPG_GetLatestEventID(t *testing.T) {
	store := newTestPGStore(t)

	// Empty table → 0
	id, err := store.GetLatestEventID()
	if err != nil {
		t.Fatalf("GetLatestEventID: %v", err)
	}
	if id != 0 {
		t.Errorf("want 0 for empty, got %d", id)
	}

	// After insert → non-zero
	now := time.Now().Unix()
	if err := store.SaveStatusEvent(&StatusEvent{
		Provider: "p", Service: "s", Channel: "c", EventType: EventTypeDown,
		TriggerRecordID: 1, ObservedAt: now, CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveStatusEvent: %v", err)
	}

	id2, err := store.GetLatestEventID()
	if err != nil {
		t.Fatalf("GetLatestEventID: %v", err)
	}
	if id2 == 0 {
		t.Error("expected non-zero after insert")
	}
}

// ===== ExportDayToWriter =====

func TestPG_ExportDayToWriter(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "exp", Service: "svc", Channel: "ch", Model: "mdl"}

	dayStart := int64(1704067200) // 2024-01-01 00:00:00 UTC
	dayEnd := dayStart + 86400

	mustSavePG(t, store, pgRec(key, dayStart+100))
	mustSavePG(t, store, pgRec(key, dayStart+200))
	mustSavePG(t, store, pgRec(key, dayEnd+100)) // next day — should not be exported

	var buf bytes.Buffer
	rowCount, err := store.ExportDayToWriter(context.Background(), dayStart, dayEnd, &buf)
	if err != nil {
		t.Fatalf("ExportDayToWriter: %v", err)
	}
	if rowCount != 2 {
		t.Errorf("want 2 rows exported, got %d", rowCount)
	}

	// Parse CSV
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	// Header + 2 data rows
	if len(records) != 3 {
		t.Errorf("want 3 CSV rows (1 header + 2 data), got %d", len(records))
	}
	if len(records) > 0 && records[0][0] != "id" {
		t.Errorf("unexpected header first column: %q", records[0][0])
	}
}

func TestPG_ExportDayToWriter_Empty(t *testing.T) {
	store := newTestPGStore(t)

	var buf bytes.Buffer
	rowCount, err := store.ExportDayToWriter(context.Background(), 1704067200, 1704067200+86400, &buf)
	if err != nil {
		t.Fatalf("ExportDayToWriter: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("want 0 rows, got %d", rowCount)
	}
}

// ===== GetTimelineAggBatch =====

func TestPG_GetTimelineAggBatch(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "agg", Service: "svc", Channel: "ch", Model: "mdl"}

	// 4 buckets of 1 hour each:
	// endTime = 2024-01-01 04:00 UTC (1704081600)
	// since   = 2024-01-01 00:00 UTC (1704067200)
	endTime := time.Unix(1704081600, 0)
	since := time.Unix(1704067200, 0)
	bucketCount := 4
	bucketWindow := time.Hour

	// Bucket 0 [00:00, 01:00): record at 00:30 → status=1, latency=200
	r0 := pgRec(key, 1704069000)
	r0.Status = 1
	r0.Latency = 200
	mustSavePG(t, store, r0)

	// Bucket 1 [01:00, 02:00): record at 01:30 → status=0, server_error
	r1 := pgRec(key, 1704072600)
	r1.Status = 0
	r1.SubStatus = SubStatusServerError
	r1.HttpCode = 500
	r1.Latency = 0
	mustSavePG(t, store, r1)

	// Bucket 2 [02:00, 03:00): no records

	// Bucket 3 [03:00, 04:00): record at 03:30 → status=2, slow_latency
	r3 := pgRec(key, 1704079800)
	r3.Status = 2
	r3.SubStatus = SubStatusSlowLatency
	r3.Latency = 5000
	mustSavePG(t, store, r3)

	got, err := store.GetTimelineAggBatch([]MonitorKey{key}, since, endTime, bucketCount, bucketWindow, nil)
	if err != nil {
		t.Fatalf("GetTimelineAggBatch: %v", err)
	}

	rows := got[key]
	if len(rows) != 3 { // buckets 0, 1, 3 (bucket 2 empty)
		t.Fatalf("want 3 bucket rows, got %d", len(rows))
	}

	// Bucket 0: available
	if rows[0].BucketIndex != 0 {
		t.Errorf("bucket 0: index=%d", rows[0].BucketIndex)
	}
	if rows[0].StatusCounts.Available != 1 {
		t.Errorf("bucket 0: available=%d", rows[0].StatusCounts.Available)
	}
	if rows[0].LatencySum != 200 || rows[0].LatencyCount != 1 {
		t.Errorf("bucket 0: latency sum=%d count=%d", rows[0].LatencySum, rows[0].LatencyCount)
	}

	// Bucket 1: unavailable + server_error
	if rows[1].BucketIndex != 1 {
		t.Errorf("bucket 1: index=%d", rows[1].BucketIndex)
	}
	if rows[1].StatusCounts.Unavailable != 1 {
		t.Errorf("bucket 1: unavailable=%d", rows[1].StatusCounts.Unavailable)
	}
	if rows[1].StatusCounts.ServerError != 1 {
		t.Errorf("bucket 1: server_error=%d", rows[1].StatusCounts.ServerError)
	}

	// Bucket 3: degraded + slow_latency
	if rows[2].BucketIndex != 3 {
		t.Errorf("bucket 3: index=%d", rows[2].BucketIndex)
	}
	if rows[2].StatusCounts.Degraded != 1 {
		t.Errorf("bucket 3: degraded=%d", rows[2].StatusCounts.Degraded)
	}
	if rows[2].StatusCounts.SlowLatency != 1 {
		t.Errorf("bucket 3: slow_latency=%d", rows[2].StatusCounts.SlowLatency)
	}
}

func TestPG_GetTimelineAggBatch_EmptyKeys(t *testing.T) {
	store := newTestPGStore(t)

	got, err := store.GetTimelineAggBatch(nil, time.Now().Add(-time.Hour), time.Now(), 4, time.Hour, nil)
	if err != nil {
		t.Fatalf("GetTimelineAggBatch empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestPG_GetTimelineAggBatch_HttpCodeBreakdown(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "hcb", Service: "svc", Channel: "ch", Model: "mdl"}

	endTime := time.Unix(1704081600, 0)
	since := time.Unix(1704078000, 0) // 1 hour before end

	// Two 500 errors and one 502 error in the same bucket
	for _, ts := range []int64{1704079000, 1704079500} {
		r := pgRec(key, ts)
		r.Status = 0
		r.SubStatus = SubStatusServerError
		r.HttpCode = 500
		mustSavePG(t, store, r)
	}
	r502 := pgRec(key, 1704080000)
	r502.Status = 0
	r502.SubStatus = SubStatusServerError
	r502.HttpCode = 502
	mustSavePG(t, store, r502)

	got, err := store.GetTimelineAggBatch([]MonitorKey{key}, since, endTime, 1, time.Hour, nil)
	if err != nil {
		t.Fatalf("GetTimelineAggBatch: %v", err)
	}

	rows := got[key]
	if len(rows) != 1 {
		t.Fatalf("want 1 bucket, got %d", len(rows))
	}

	bd := rows[0].StatusCounts.HttpCodeBreakdown
	if bd == nil {
		t.Fatal("expected http_code_breakdown")
	}
	serverErrors := bd["server_error"]
	if serverErrors == nil {
		t.Fatal("expected server_error in breakdown")
	}
	if serverErrors[500] != 2 {
		t.Errorf("want 500→2, got %d", serverErrors[500])
	}
	if serverErrors[502] != 1 {
		t.Errorf("want 502→1, got %d", serverErrors[502])
	}
}

func TestPG_GetTimelineAggBatch_TimeFilter(t *testing.T) {
	store := newTestPGStore(t)
	key := MonitorKey{Provider: "tf", Service: "svc", Channel: "ch", Model: "mdl"}

	// Single bucket spanning 2 hours
	endTime := time.Unix(1704074400, 0) // 02:00 UTC
	since := time.Unix(1704067200, 0)   // 00:00 UTC
	bucketCount := 1
	bucketWindow := 2 * time.Hour

	// Record at 00:30 UTC (30 min into day) → should be included by filter [0, 60)
	mustSavePG(t, store, pgRec(key, 1704069000))
	// Record at 01:30 UTC (90 min into day) → should be excluded by filter [0, 60)
	mustSavePG(t, store, pgRec(key, 1704072600))

	tf := &DailyTimeFilter{StartMinutes: 0, EndMinutes: 60, CrossMidnight: false}
	got, err := store.GetTimelineAggBatch([]MonitorKey{key}, since, endTime, bucketCount, bucketWindow, tf)
	if err != nil {
		t.Fatalf("GetTimelineAggBatch with filter: %v", err)
	}

	rows := got[key]
	if len(rows) != 1 {
		t.Fatalf("want 1 bucket, got %d", len(rows))
	}
	if rows[0].Total != 1 {
		t.Errorf("want 1 record after time filter, got %d", rows[0].Total)
	}
}

// ===== WithContext =====

func TestPG_WithContext(t *testing.T) {
	store := newTestPGStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	ctxStore := store.WithContext(ctx)

	key := MonitorKey{Provider: "pc", Service: "sc", Channel: "cc", Model: "mc"}
	if err := ctxStore.SaveRecord(pgRec(key, 1000)); err != nil {
		t.Fatalf("SaveRecord with context: %v", err)
	}

	cancel()

	err := ctxStore.SaveRecord(pgRec(key, 2000))
	if err == nil {
		t.Log("SaveRecord after cancel did not error (pool may reuse connections)")
	}
}

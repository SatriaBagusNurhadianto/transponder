package sqlite

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/raven-clown/transponder/internal/store"
)

func record(i int64, org string) store.UsageRecord {
	return store.UsageRecord{
		OrgID:            org,
		Time:             time.UnixMilli(1_700_000_000_000 + i*1000),
		RequestID:        "req",
		User:             "alice",
		Teams:            []string{"platform"},
		Key:              "alice-key",
		Project:          "demo",
		Route:            "claude-*",
		Provider:         "echo",
		Model:            "m",
		Endpoint:         "chat.completions",
		InputTokens:      10 + i,
		OutputTokens:     20,
		CacheReadTokens:  3,
		CacheWriteTokens: 4,
		CostNUSD:         4542000,
		Estimated:        true,
		Status:           200,
		Latency:          150 * time.Millisecond,
		Fallbacks:        1,
	}
}

func TestRecordAndRecentUsage(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for i := int64(1); i <= 3; i++ {
		if err := db.RecordUsage(ctx, record(i, store.DefaultOrg)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordUsage(ctx, record(9, "acme")); err != nil {
		t.Fatal(err)
	}
	got, err := db.RecentUsage(ctx, store.DefaultOrg, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []store.UsageRecord{record(3, store.DefaultOrg), record(2, store.DefaultOrg)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestRecordUsageUnderLoad(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	const n = 200
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { errs <- db.RecordUsage(ctx, record(int64(i), store.DefaultOrg)) })
	}
	wg.Wait()
	close(errs)
	failed := 0
	for err := range errs {
		if err != nil {
			failed++
			t.Log(err)
		}
	}
	if failed > 0 {
		t.Fatalf("%d of %d writes failed", failed, n)
	}
	recs, err := db.RecentUsage(ctx, store.DefaultOrg, n+1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != n {
		t.Fatalf("records = %d, want %d", len(recs), n)
	}
}

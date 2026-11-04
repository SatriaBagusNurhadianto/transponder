package sqlite

import (
	"context"
	"reflect"
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

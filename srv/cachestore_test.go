package srv

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

func TestCacheStoreRoundtrip(t *testing.T) {
	f, _ := os.CreateTemp("", "cs*.sqlite3")
	f.Close()
	defer os.Remove(f.Name())
	wdb, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(wdb); err != nil {
		t.Fatal(err)
	}
	q := Store{dbgen.New(wdb)}
	ctx := context.Background()
	big := `{"parcels":[` + strings.Repeat(`{"a":"äöü€","n":1},`, 5000) + `{}]}`
	for _, tc := range []struct{ k, v string }{{"small", `{"x":1}`}, {"big", big}} {
		if err := q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: tc.k, Data: tc.v, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		got, err := q.GetCachedData(ctx, tc.k)
		if err != nil || got != tc.v {
			t.Fatalf("%s: roundtrip mismatch err=%v len=%d/%d", tc.k, err, len(got), len(tc.v))
		}
	}
	var raw string
	wdb.QueryRow(`SELECT data FROM api_cache WHERE cache_key='big'`).Scan(&raw)
	if !isGzipped(raw) || len(raw) > len(big)/5 {
		t.Fatalf("big row not compressed: gz=%v len=%d", isGzipped(raw), len(raw))
	}
	wdb.QueryRow(`SELECT data FROM api_cache WHERE cache_key='small'`).Scan(&raw)
	if isGzipped(raw) {
		t.Fatal("small row should stay plain")
	}
}

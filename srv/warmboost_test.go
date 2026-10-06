package srv

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWarmBoostFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "WARM_BOOST")
	t.Setenv("SIEDLER_WARM_BOOST", f)
	reset := func() { boostCache.Lock(); boostCache.at = time.Time{}; boostCache.Unlock() }

	reset()
	if warmBoosted() {
		t.Fatal("no file → not boosted")
	}
	os.WriteFile(f, []byte(time.Now().AddDate(0, 0, 2).Format("2006-01-02")+"\n"), 0o644)
	reset()
	if !warmBoosted() || warmTier() != "boost" || warmIdle() || warmPatchesNow() != warmBoostPatches {
		t.Fatalf("date file → boosted, got tier=%s patches=%d", warmTier(), warmPatchesNow())
	}
	os.WriteFile(f, []byte("2001-01-01"), 0o644)
	reset()
	if warmBoosted() {
		t.Fatal("past date → not boosted")
	}
	os.WriteFile(f, nil, 0o644)
	reset()
	if u := warmBoostUntil(); !warmBoosted() || time.Until(u) < 47*time.Hour {
		t.Fatalf("empty file → mtime+48h, got %v", u)
	}
}

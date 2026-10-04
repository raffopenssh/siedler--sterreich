package srv

import (
	"testing"
	"time"
)

func TestTileThrottleBackoff(t *testing.T) {
	tileGate.mu.Lock()
	tileGate.backoff, tileGate.pauseUntil = 0, time.Time{}
	tileGate.mu.Unlock()
	if d := tileThrottle(503, ""); d != time.Second {
		t.Fatalf("first pause without Retry-After = %s, want 1s", d)
	}
	if d := tileThrottle(503, ""); d != 2*time.Second {
		t.Fatalf("second pause = %s, want 2s", d)
	}
	if d := tileThrottle(429, "7"); d != 7*time.Second {
		t.Fatalf("Retry-After 7 → %s", d)
	}
	for i := 0; i < 6; i++ {
		tileThrottle(503, "")
	}
	if d := tileThrottle(503, ""); d != tileBackoffMax {
		t.Fatalf("cap = %s, want %s", d, tileBackoffMax)
	}
	if tilePaused() <= 0 {
		t.Fatal("gate should be paused")
	}
	tileRecovered()
	if d := tileThrottle(503, "90"); d != tileBackoffMax {
		t.Fatalf("Retry-After above cap → %s", d)
	}
	tileGate.mu.Lock()
	tileGate.backoff, tileGate.pauseUntil = 0, time.Time{}
	tileGate.mu.Unlock()
}

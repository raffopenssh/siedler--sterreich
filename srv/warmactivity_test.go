package srv

import (
	"testing"
	"time"
)

func TestWarmTier(t *testing.T) {
	defer lastPlayer.Store(lastPlayer.Load())
	lastPlayer.Store(0)
	if !warmIdle() || warmTier() != "idle" {
		t.Fatal("no activity ever → idle")
	}
	playerSeen()
	if warmIdle() || warmTier() != "active" {
		t.Fatal("just seen → active")
	}
	lastPlayer.Store(time.Now().Add(-warmIdleAfter - time.Minute).Unix())
	if !warmIdle() {
		t.Fatal("older than warmIdleAfter → idle")
	}
	if warmPatches%warmIdleEvery != 0 {
		t.Fatalf("idle cadence %d must divide %d patches", warmIdleEvery, warmPatches)
	}
}

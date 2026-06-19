package batch

import (
	"sync"
	"testing"
	"time"
)

func TestNewAdaptivePool_DefaultBounds(t *testing.T) {
	ap := NewAdaptivePool(0, 0)
	if ap.minPerHost != 2 {
		t.Errorf("minPerHost = %d, want 2", ap.minPerHost)
	}
	if ap.maxPerHost != 8 {
		t.Errorf("maxPerHost = %d, want 8", ap.maxPerHost)
	}
}

func TestNewAdaptivePool_CustomBounds(t *testing.T) {
	ap := NewAdaptivePool(3, 16)
	if ap.minPerHost != 3 {
		t.Errorf("minPerHost = %d, want 3", ap.minPerHost)
	}
	if ap.maxPerHost != 16 {
		t.Errorf("maxPerHost = %d, want 16", ap.maxPerHost)
	}
}

func TestGetConcurrency_NewHost(t *testing.T) {
	ap := NewAdaptivePool(4, 12)
	got := ap.GetConcurrency("example.com")
	if got != 4 {
		t.Errorf("GetConcurrency(new host) = %d, want 4 (minPerHost)", got)
	}
}

func TestRecordSuccess_ScaleUp(t *testing.T) {
	ap := NewAdaptivePool(2, 10)
	host := "example.com"

	// Initial concurrency
	if ap.GetConcurrency(host) != 2 {
		t.Fatal("expected initial concurrency 2")
	}

	// 5 successes with no errors → scale up
	for i := 0; i < 5; i++ {
		ap.RecordSuccess(host, 10*time.Millisecond)
	}

	if got := ap.GetConcurrency(host); got != 3 {
		t.Errorf("after 5 successes: concurrency = %d, want 3", got)
	}
}

func TestRecordSuccess_NoScaleUp_WithErrors(t *testing.T) {
	ap := NewAdaptivePool(2, 10)
	host := "example.com"

	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordFailure(host) // introduces an error
	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordSuccess(host, 10*time.Millisecond)

	// Should not scale up because there was an error
	if got := ap.GetConcurrency(host); got != 2 {
		t.Errorf("concurrency = %d, want 2 (no scale up with errors)", got)
	}
}

func TestRecordSuccess_MaxCap(t *testing.T) {
	ap := NewAdaptivePool(8, 10)
	host := "example.com"

	// Already at max, 5 successes should not exceed max
	for i := 0; i < 10; i++ {
		ap.RecordSuccess(host, 10*time.Millisecond)
	}

	if got := ap.GetConcurrency(host); got != 10 {
		t.Errorf("concurrency = %d, want 10 (max cap)", got)
	}
}

func TestRecordFailure_ScaleDown(t *testing.T) {
	ap := NewAdaptivePool(2, 10)
	host := "example.com"

	// Scale up first
	for i := 0; i < 5; i++ {
		ap.RecordSuccess(host, 10*time.Millisecond)
	}
	if ap.GetConcurrency(host) != 3 {
		t.Fatal("setup: expected concurrency 3")
	}

	// 2 failures → scale down
	ap.RecordFailure(host)
	ap.RecordFailure(host)

	if got := ap.GetConcurrency(host); got != 2 {
		t.Errorf("after 2 failures: concurrency = %d, want 2", got)
	}
}

func TestRecordFailure_MinFloor(t *testing.T) {
	ap := NewAdaptivePool(3, 10)
	host := "example.com"

	// Already at min, 2 failures should not go below
	ap.RecordFailure(host)
	ap.RecordFailure(host)

	if got := ap.GetConcurrency(host); got != 3 {
		t.Errorf("concurrency = %d, want 3 (min floor)", got)
	}
}

func TestRecordFailure_ResetsSuccessCount(t *testing.T) {
	ap := NewAdaptivePool(2, 10)
	host := "example.com"

	// 3 successes, then a failure resets the count
	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordFailure(host)

	// Need 5 more successes to scale up (count was reset)
	for i := 0; i < 4; i++ {
		ap.RecordSuccess(host, 10*time.Millisecond)
	}

	// Should still be at min (only 4 successes since reset)
	if got := ap.GetConcurrency(host); got != 2 {
		t.Errorf("concurrency = %d, want 2 (failure reset success count)", got)
	}
}

func TestReset(t *testing.T) {
	ap := NewAdaptivePool(2, 10)
	host := "example.com"

	ap.RecordSuccess(host, 10*time.Millisecond)
	ap.RecordSuccess(host, 10*time.Millisecond)

	ap.Reset(host)

	// After reset, should get fresh stats with minPerHost
	if got := ap.GetConcurrency(host); got != 2 {
		t.Errorf("after Reset: concurrency = %d, want 2", got)
	}
}

func TestMultipleHosts_Independent(t *testing.T) {
	ap := NewAdaptivePool(2, 10)

	// Scale up host A
	for i := 0; i < 5; i++ {
		ap.RecordSuccess("a.com", 10*time.Millisecond)
	}

	// Scale down host B
	ap.RecordFailure("b.com")
	ap.RecordFailure("b.com")

	if got := ap.GetConcurrency("a.com"); got != 3 {
		t.Errorf("host A concurrency = %d, want 3", got)
	}
	if got := ap.GetConcurrency("b.com"); got != 2 {
		t.Errorf("host B concurrency = %d, want 2", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	ap := NewAdaptivePool(2, 10)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			host := "example.com"
			for j := 0; j < 50; j++ {
				ap.GetConcurrency(host)
				ap.RecordSuccess(host, time.Millisecond)
				ap.RecordFailure(host)
			}
		}(i)
	}

	wg.Wait()

	// No panic or race = success (run with -race)
	c := ap.GetConcurrency("example.com")
	if c < 2 || c > 10 {
		t.Errorf("concurrency %d out of expected range [2, 10]", c)
	}
}

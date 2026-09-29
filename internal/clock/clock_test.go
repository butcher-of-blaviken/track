package clock_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/clock"
)

var start = time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)

func TestFake_NowReturnsStartAndDoesNotDrift(t *testing.T) {
	f := clock.NewFake(start)
	if got := f.Now(); !got.Equal(start) {
		t.Errorf("Now() = %v, want %v", got, start)
	}
	time.Sleep(5 * time.Millisecond)
	if got := f.Now(); !got.Equal(start) {
		t.Errorf("Now() drifted to %v, want %v", got, start)
	}
}

func TestFake_AdvanceMovesForwardAndAccumulates(t *testing.T) {
	f := clock.NewFake(start)
	f.Advance(30 * time.Minute)
	f.Advance(90 * time.Second)
	want := time.Date(2026, 9, 29, 9, 31, 30, 0, time.UTC)
	if got := f.Now(); !got.Equal(want) {
		t.Errorf("Now() = %v, want %v", got, want)
	}
}

func TestFake_SetJumpsIncludingBackwards(t *testing.T) {
	f := clock.NewFake(start)
	later := start.Add(48 * time.Hour)
	f.Set(later)
	if got := f.Now(); !got.Equal(later) {
		t.Errorf("Now() = %v, want %v", got, later)
	}
	earlier := start.Add(-time.Hour)
	f.Set(earlier)
	if got := f.Now(); !got.Equal(earlier) {
		t.Errorf("Now() = %v, want %v after moving backwards", got, earlier)
	}
}

func TestFake_IsSafeForConcurrentUse(t *testing.T) {
	f := clock.NewFake(start)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			f.Advance(time.Second)
		}
	}()
	for i := 0; i < 1000; i++ {
		_ = f.Now()
	}
	<-done
	if got, want := f.Now(), start.Add(1000*time.Second); !got.Equal(want) {
		t.Errorf("Now() = %v, want %v", got, want)
	}
}

func TestSystem_NowIsTheRealTime(t *testing.T) {
	before := time.Now()
	got := clock.System{}.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("Now() = %v, want between %v and %v", got, before, after)
	}
}

// Both implementations satisfy Clock.
var (
	_ clock.Clock = clock.System{}
	_ clock.Clock = (*clock.Fake)(nil)
)

func TestScaled_RunsFasterThanItsBaseFromTheMomentOfCreation(t *testing.T) {
	base := clock.NewFake(start)
	s, err := clock.NewScaled(base, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Now(); !got.Equal(start) {
		t.Errorf("Now() at creation = %v, want the base's time %v", got, start)
	}

	base.Advance(10 * time.Second)
	if got, want := s.Now(), start.Add(10*time.Minute); !got.Equal(want) {
		t.Errorf("Now() after 10s of base time = %v, want %v (60x)", got, want)
	}
	base.Advance(time.Second)
	if got, want := s.Now(), start.Add(11*time.Minute); !got.Equal(want) {
		t.Errorf("Now() after 11s of base time = %v, want %v", got, want)
	}
}

func TestScaled_FactorOneFollowsTheBaseAndFractionsSlowItDown(t *testing.T) {
	base := clock.NewFake(start)
	one, _ := clock.NewScaled(base, 1)
	half, _ := clock.NewScaled(base, 0.5)
	base.Advance(40 * time.Second)
	if got := one.Now(); !got.Equal(base.Now()) {
		t.Errorf("factor 1: Now() = %v, want %v", got, base.Now())
	}
	if got, want := half.Now(), start.Add(20*time.Second); !got.Equal(want) {
		t.Errorf("factor 0.5: Now() = %v, want %v", got, want)
	}
}

func TestNewScaled_RejectsANonPositiveOrNonFiniteFactor(t *testing.T) {
	for _, factor := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := clock.NewScaled(clock.NewFake(start), factor); !errors.Is(err, clock.ErrInvalidScale) {
			t.Errorf("NewScaled(%v) error = %v, want ErrInvalidScale", factor, err)
		}
	}
}

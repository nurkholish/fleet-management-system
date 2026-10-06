package memory

import (
	"context"
	"sync"
	"testing"

	"fleet-management-system/internal/domain"
)

// Compile-time interface conformance check.
var _ domain.GeofenceStateStore = (*GeofenceState)(nil)

const (
	testVehicle  = "B1234XYZ"
	testVehicle2 = "B5678ABC"
	testFence    = "monas"
	testFence2   = "blok_m"
)

// ---------- Constructor ----------

func TestNewGeofenceState(t *testing.T) {
	s := NewGeofenceState()
	if s == nil {
		t.Fatal("expected non-nil state")
	}
	if s.inside == nil {
		t.Fatal("expected non-nil internal map")
	}
	if len(s.inside) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(s.inside))
	}
}

// ---------- EnterIfOutside ----------

func TestEnterIfOutside_FirstCallTrue(t *testing.T) {
	s := NewGeofenceState()
	entered, err := s.EnterIfOutside(context.Background(), testVehicle, testFence)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !entered {
		t.Fatal("first enter should be true")
	}
}

func TestEnterIfOutside_SecondCallFalse(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	first, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if !first {
		t.Fatal("first enter should be true")
	}
	second, err := s.EnterIfOutside(ctx, testVehicle, testFence)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second {
		t.Fatal("second enter should be false (already inside)")
	}
}

func TestEnterIfOutside_IdempotentWhileInside(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	s.EnterIfOutside(ctx, testVehicle, testFence)
	for i := 0; i < 10; i++ {
		again, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
		if again {
			t.Fatalf("call #%d should be false while already inside", i+2)
		}
	}
}

// ---------- ExitIfInside ----------

func TestExitIfInside_AfterEnterTrue(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	s.EnterIfOutside(ctx, testVehicle, testFence)
	exited, err := s.ExitIfInside(ctx, testVehicle, testFence)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exited {
		t.Fatal("exit after enter should be true")
	}
}

func TestExitIfInside_WithoutEnterFalse(t *testing.T) {
	s := NewGeofenceState()
	exited, err := s.ExitIfInside(context.Background(), testVehicle, testFence)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exited {
		t.Fatal("exit without enter should be false")
	}
}

func TestExitIfInside_TwiceSecondFalse(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	s.EnterIfOutside(ctx, testVehicle, testFence)

	first, _ := s.ExitIfInside(ctx, testVehicle, testFence)
	if !first {
		t.Fatal("first exit should be true")
	}
	second, _ := s.ExitIfInside(ctx, testVehicle, testFence)
	if second {
		t.Fatal("second exit should be false")
	}
}

// ---------- Full cycle: enter → exit → enter ----------

func TestFullCycle_EnterExitEnter(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	e1, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if !e1 {
		t.Fatal("entry #1 should be true")
	}

	repeat, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if repeat {
		t.Fatal("repeat enter while inside should be false")
	}

	x, _ := s.ExitIfInside(ctx, testVehicle, testFence)
	if !x {
		t.Fatal("exit should be true")
	}

	e2, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if !e2 {
		t.Fatal("entry #2 after exit should be true")
	}
}

// ---------- Reset ----------

func TestReset_MakesEnterableAgain(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	s.EnterIfOutside(ctx, testVehicle, testFence)

	// while inside, repeat enter is false
	repeat, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if repeat {
		t.Fatal("repeat enter should be false before reset")
	}

	s.Reset(ctx, testVehicle, testFence)

	entered, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if !entered {
		t.Fatal("enter after reset should be true")
	}
}

func TestReset_OnEmptyNoPanic(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	s.Reset(ctx, testVehicle, testFence) // no prior enter

	entered, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if !entered {
		t.Fatal("enter after reset-on-empty should be true")
	}
}

// ---------- Multiple vehicles ----------

func TestMultipleVehicles_IndependentState(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	a, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	b, _ := s.EnterIfOutside(ctx, testVehicle2, testFence)

	if !a || !b {
		t.Fatal("both vehicles should enter independently")
	}

	a2, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	b2, _ := s.EnterIfOutside(ctx, testVehicle2, testFence)
	if a2 {
		t.Fatal("vehicle A repeat should be false")
	}
	if b2 {
		t.Fatal("vehicle B repeat should be false")
	}
}

func TestMultipleVehicles_ExitOneDoesNotAffectOther(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	s.EnterIfOutside(ctx, testVehicle, testFence)
	s.EnterIfOutside(ctx, testVehicle2, testFence)

	s.ExitIfInside(ctx, testVehicle, testFence)

	a, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if !a {
		t.Fatal("vehicle A should re-enter after its own exit")
	}
	b, _ := s.EnterIfOutside(ctx, testVehicle2, testFence)
	if b {
		t.Fatal("vehicle B should still be inside")
	}
}

// ---------- Multiple geofences ----------

func TestMultipleGeofences_IndependentState(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	m, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	b, _ := s.EnterIfOutside(ctx, testVehicle, testFence2)

	if !m || !b {
		t.Fatal("both fences should trigger independently")
	}

	m2, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	b2, _ := s.EnterIfOutside(ctx, testVehicle, testFence2)
	if m2 {
		t.Fatal("monas repeat should be false")
	}
	if b2 {
		t.Fatal("blok_m repeat should be false")
	}
}

func TestMultipleGeofences_ExitOneDoesNotAffectOther(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	s.EnterIfOutside(ctx, testVehicle, testFence)
	s.EnterIfOutside(ctx, testVehicle, testFence2)

	s.ExitIfInside(ctx, testVehicle, testFence)

	m, _ := s.EnterIfOutside(ctx, testVehicle, testFence)
	if !m {
		t.Fatal("monas should be re-enterable")
	}
	b, _ := s.EnterIfOutside(ctx, testVehicle, testFence2)
	if b {
		t.Fatal("blok_m should still be inside")
	}
}

// ---------- Concurrency ----------

func TestConcurrentAccess_NoRace(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				s.EnterIfOutside(ctx, testVehicle, testFence)
				s.ExitIfInside(ctx, testVehicle, testFence)
				s.Reset(ctx, testVehicle, testFence)
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentAccess_DistinctKeys(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	const workers = 50
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			vid := testVehicle + "-" + string(rune('A'+id%26))
			for j := 0; j < 50; j++ {
				s.EnterIfOutside(ctx, vid, testFence)
				s.ExitIfInside(ctx, vid, testFence)
			}
		}(i)
	}
	wg.Wait()
}

// ---------- Context handling ----------

func TestContextCancelled_StillOperates(t *testing.T) {
	s := NewGeofenceState()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entered, err := s.EnterIfOutside(ctx, testVehicle, testFence)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !entered {
		t.Fatal("cancelled ctx should not affect operation")
	}
}

// ---------- Interface conformance (runtime smoke) ----------

func TestImplementsGeofenceStateStore(t *testing.T) {
	var store domain.GeofenceStateStore = NewGeofenceState()
	ctx := context.Background()

	entered, err := store.EnterIfOutside(ctx, testVehicle, testFence)
	if err != nil || !entered {
		t.Fatal("interface EnterIfOutside failed")
	}
	exited, err := store.ExitIfInside(ctx, testVehicle, testFence)
	if err != nil || !exited {
		t.Fatal("interface ExitIfInside failed")
	}
	store.Reset(ctx, testVehicle, testFence) // must not panic
}

// ---------- Edge cases ----------

func TestEdgeCase_EmptyStrings(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	entered, _ := s.EnterIfOutside(ctx, "", "")
	if !entered {
		t.Fatal("empty keys should work")
	}
	again, _ := s.EnterIfOutside(ctx, "", "")
	if again {
		t.Fatal("repeat with empty keys should be false")
	}
	s.Reset(ctx, "", "")
}

func TestEdgeCase_SpecialCharacters(t *testing.T) {
	s := NewGeofenceState()
	ctx := context.Background()

	weird := "B-123|XYZ/!!"
	fence := "fence with spaces"

	entered, _ := s.EnterIfOutside(ctx, weird, fence)
	if !entered {
		t.Fatal("special chars should work")
	}
	s.ExitIfInside(ctx, weird, fence)
	s.Reset(ctx, weird, fence)
}

func TestEdgeCase_NulByteSeparatorPreventsCollision(t *testing.T) {
	// Production code uses "\x00" as separator, so IDs containing
	// printable characters (including '|') never collide.
	s := NewGeofenceState()
	ctx := context.Background()

	a, _ := s.EnterIfOutside(ctx, "a|b", "c")
	b, _ := s.EnterIfOutside(ctx, "a", "b|c")

	if !a {
		t.Fatal("first composite key should enter")
	}
	if !b {
		t.Fatal("second composite key should also enter (distinct)")
	}

	a2, _ := s.EnterIfOutside(ctx, "a|b", "c")
	b2, _ := s.EnterIfOutside(ctx, "a", "b|c")
	if a2 {
		t.Fatal("first repeat should be false")
	}
	if b2 {
		t.Fatal("second repeat should be false")
	}
}

// ---------- Benchmarks ----------

func BenchmarkEnterIfOutside(b *testing.B) {
	s := NewGeofenceState()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.EnterIfOutside(ctx, testVehicle, testFence)
		s.ExitIfInside(ctx, testVehicle, testFence)
	}
}

func BenchmarkEnterIfOutside_Parallel(b *testing.B) {
	s := NewGeofenceState()
	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.EnterIfOutside(ctx, testVehicle, testFence)
			s.ExitIfInside(ctx, testVehicle, testFence)
		}
	})
}

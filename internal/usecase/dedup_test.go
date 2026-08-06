package usecase

import "testing"

func TestDedupCache(t *testing.T) {
	t.Parallel()

	d := newDedupCache(100)

	if d.isDuplicate("a", 1) {
		t.Fatal("first occurrence must not be a duplicate")
	}
	if !d.isDuplicate("a", 1) {
		t.Fatal("same uid and count must be a duplicate")
	}
	if !d.isDuplicate("a", 0) {
		t.Fatal("lower count for a known uid must be a duplicate")
	}
	if d.isDuplicate("a", 2) {
		t.Fatal("higher count is a new occurrence, not a duplicate")
	}
	if d.isDuplicate("b", 1) {
		t.Fatal("unknown uid must not be a duplicate")
	}
}

func TestDedupCacheRotation(t *testing.T) {
	t.Parallel()

	d := newDedupCache(2)

	d.isDuplicate("a", 1) // cur: {a}
	d.isDuplicate("b", 1) // cur: {a, b} — full
	d.isDuplicate("c", 1) // rotate: prev: {a, b}, cur: {c}

	if !d.isDuplicate("a", 1) {
		t.Fatal("entry from previous generation must still be found")
	}

	// The duplicate lookup above promoted "a" into cur ({c, a}), while "b"
	// stayed only in prev. Two more rotations age "b" out completely.
	d.isDuplicate("d", 1) // rotate: prev: {c, a}, cur: {d}
	d.isDuplicate("e", 1) // cur: {d, e}
	d.isDuplicate("f", 1) // rotate: prev: {d, e}, cur: {f}

	if d.isDuplicate("b", 1) {
		t.Fatal("entry two generations old must be forgotten")
	}
}

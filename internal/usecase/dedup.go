// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package usecase

// dedupCache remembers the highest count seen per event UID, so that watch
// re-deliveries after reconnects do not export the same event twice.
// Memory is bounded by two generations of maxPerGen entries each: when the
// current generation fills up, it replaces the previous one and older entries
// are forgotten. Not safe for concurrent use; Collector.Run is the only caller.
//
// Trade-off: entries are recorded before delivery is confirmed, so an event
// whose batch is later dropped (permanent rejection, shutdown overflow) is not
// re-exported when the watch re-delivers it. This keeps the pipeline a simple
// single pass; delivery-acknowledged dedup is a possible future extension.
type dedupCache struct {
	maxPerGen int
	cur       map[string]int32
	prev      map[string]int32
}

func newDedupCache(maxPerGen int) *dedupCache {
	return &dedupCache{
		maxPerGen: maxPerGen,
		cur:       make(map[string]int32),
		prev:      make(map[string]int32),
	}
}

// isDuplicate reports whether this (uid, count) pair was already seen and
// records it otherwise. A higher count for a known UID is a new occurrence
// of a recurring event, not a duplicate.
func (d *dedupCache) isDuplicate(uid string, count int32) bool {
	if last, ok := d.lookup(uid); ok && count <= last {
		d.store(uid, last) // refresh, so recurring UIDs do not age out
		return true
	}
	d.store(uid, count)
	return false
}

func (d *dedupCache) lookup(uid string) (int32, bool) {
	if v, ok := d.cur[uid]; ok {
		return v, true
	}
	if v, ok := d.prev[uid]; ok {
		return v, true
	}
	return 0, false
}

func (d *dedupCache) store(uid string, count int32) {
	if _, exists := d.cur[uid]; !exists && len(d.cur) >= d.maxPerGen {
		d.prev = d.cur
		d.cur = make(map[string]int32, d.maxPerGen)
	}
	d.cur[uid] = count
}

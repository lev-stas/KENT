package kubernetes

import "time"

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func toSet(list []string) map[string]struct{} {
	set := make(map[string]struct{}, len(list))
	for _, v := range list {
		set[v] = struct{}{}
	}
	return set
}

package vcs

import (
	"sort"
	"time"
)

type Thread []Comment

func (t Thread) Lines() (lo, hi int) {
	if len(t) == 0 {
		return 0, 0
	}
	return t[0].Lines()
}

func (t Thread) CreatedAt() time.Time {
	if len(t) == 0 {
		return time.Time{}
	}
	return t[0].CreatedAt()
}

func (t Thread) Side() Side {
	if len(t) == 0 {
		return SideUnknown
	}
	return t[0].Side()
}

func BuildThreads(cs []Comment) (ts []Thread) {
	index := make(map[string]Thread)
	for _, c := range cs {
		var id string
		if p := c.Parent(); p != nil {
			id = p.ID()
		} else {
			id = c.ID()
		}
		index[id] = append(index[id], c)
	}
	ts = make([]Thread, 0, len(index))
	for _, t := range index {
		sort.Slice(t, func(i, j int) bool {
			t0 := t[i].CreatedAt()
			t1 := t[j].CreatedAt()
			return t0.Before(t1)
		})
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool {
		start0, _ := ts[i].Lines()
		start1, _ := ts[j].Lines()
		return start0 < start1 || ts[i].CreatedAt().Before(ts[j].CreatedAt())
	})
	return ts
}

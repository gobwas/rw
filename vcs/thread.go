package vcs

import (
	"sort"
)

type node struct {
	comment Comment
	next    *node
}

func (n *node) less(b *node) bool {
	return n.comment.Compare(b.comment) < 0
}

type thread struct {
	head *node
	tail *node
}

func (t thread) isZero() bool {
	return t.head == nil
}

func (t thread) push(n *node) thread {
	if t.head == nil {
		t.head = n
		t.tail = n
	} else {
		t.tail.next = n
		t.tail = n
	}
	return t
}

func (t thread) pushComment(c Comment) thread {
	return t.push(&node{
		comment: c,
	})
}

type Thread []Comment

func BuildThreads(cs []Comment) (ts []Thread) {
	index := make(map[string]thread)
	for _, c := range cs {
		if p := c.Parent(); p != nil {
			id := p.ID()
			t, has := index[id]
			if !has {
				t = t.pushComment(p)
			}
			index[id] = t.pushComment(c)
			continue
		} else {
			id := c.ID()
			index[id] = index[id].pushComment(c)
		}
	}

	ts = make([]Thread, 0, len(index))
	for _, t := range index {
		head := mergeSort(t.head)
		var cs []Comment
		for el := head; el != nil; el = el.next {
			cs = append(cs, el.comment)
		}
		ts = append(ts, Thread(cs))
	}
	sort.Slice(ts, func(i, j int) bool {
		c1 := ts[i][0]
		c2 := ts[j][0]
		return c1.Line() < c2.Line() || c1.CreatedAt().Before(c2.CreatedAt())
	})

	return ts
}

func mergeSort(head *node) *node {
	if head == nil || head.next == nil {
		return head
	}
	lo, hi := split(head)
	return merge(
		mergeSort(lo),
		mergeSort(hi),
	)
}

func split(head *node) (a, b *node) {
	var slow, fast *node
	for slow, fast = head, head.next; fast != nil && fast.next != nil; {
		fast = fast.next.next
		slow = slow.next
	}
	tail := slow.next
	slow.next = nil
	return head, tail
}

func merge(a, b *node) (r *node) {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.less(b) {
		a.next = merge(a.next, b)
		r = a
	} else {
		b.next = merge(b.next, a)
		r = b
	}
	return r
}

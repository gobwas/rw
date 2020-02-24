package listutil

import (
	"container/list"
)

func InsertInOrder(l *list.List, v interface{}, less func(a, b interface{}) bool) *list.Element {
	m := l.Front()
	for ; m != nil && less(m.Value, v); m = m.Next() {
	}
	if m == nil {
		return l.PushBack(v)
	}
	return l.InsertBefore(v, m)
}

func Sort(l *list.List, less func(a, b interface{}) bool) {
	front := mergeSort(l.Front(), less)
	var back *list.Element
	for el := front; el != nil; back, el = el, el.Next() {
		// Fix prev links.
		storeLinks(el, back, el.Next())
	}
	storeLinks(listRoot(l), back, front)
}

func mergeSort(el *list.Element, less func(a, b interface{}) bool) *list.Element {
	if el == nil || el.Next() == nil {
		return el
	}
	lo, hi := split(el)
	return merge(
		mergeSort(lo, less),
		mergeSort(hi, less),
		less,
	)
}

func split(el *list.Element) (lo, hi *list.Element) {
	var slow, fast *list.Element
	for slow, fast = el, el.Next(); fast != nil && fast.Next() != nil; {
		slow = slow.Next()
		fast = fast.Next().Next()
	}
	// slow is now the middle of the list.
	lo = el
	hi = slow.Next()
	updateNext(slow, nil)
	return
}

func merge(a, b *list.Element, less func(a, b interface{}) bool) (ret *list.Element) {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if less(a.Value, b.Value) {
		updateNext(a, merge(a.Next(), b, less))
		ret = a
	} else {
		updateNext(b, merge(b.Next(), a, less))
		ret = b
	}
	return ret
}

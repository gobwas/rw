package listutil

import (
	"container/list"
	"unsafe"
)

type listElement struct {
	next, prev *list.Element
}

type listList struct {
	root list.Element
}

func storeLinks(el, prev, next *list.Element) {
	e := (*listElement)(unsafe.Pointer(el))
	e.next = next
	e.prev = prev
}

func updateNext(el, new *list.Element) {
	if old := el.Next(); old != nil {
		storeLinks(old, nil, old.Next())
	}
	if new != nil {
		storeLinks(new, el, new.Next())
	}
	storeLinks(el, el.Prev(), new)
}

func updatePrev(el, new *list.Element) {
	if old := el.Prev(); old != nil {
		storeLinks(old, old.Prev(), nil)
	}
	if new != nil {
		storeLinks(new, new.Prev(), el)
	}
	storeLinks(el, new, el.Next())
}

func listRoot(list *list.List) *list.Element {
	l := (*listList)(unsafe.Pointer(list))
	return &l.root
}

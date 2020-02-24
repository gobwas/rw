package listutil

import (
	"container/list"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func BenchmarkSort(b *testing.B) {
	for _, test := range []struct {
		name string
		ints []int
		perm bool
	}{
		{
			ints: []int{11, 5, 4, 1, 2, 3, 7, 6, 12, 9, 10, 8},
			perm: false,
		},
	} {
		var xss [][]int
		if test.perm {
			xss = perm(test.ints)
		} else {
			xss = [][]int{test.ints}
		}
		for _, xs := range xss {
			name := fmt.Sprintf("%v", xs)

			b.Run(name+"-slice", func(b *testing.B) {
				cps := make([][]int, b.N)
				for i := 0; i < b.N; i++ {
					cps[i] = append(([]int)(nil), xs...)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sort.Ints(cps[i])
				}
			})
			b.Run(name+"-list", func(b *testing.B) {
				ls := make([]*list.List, b.N)
				for i := 0; i < b.N; i++ {
					ls[i] = buildList(xs...)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					Sort(ls[i], func(a, b interface{}) bool {
						return a.(int) < b.(int)
					})
				}
			})
		}
	}
}

func TestSort(t *testing.T) {
	for _, test := range []struct {
		name string
		ints []int
		perm bool
	}{
		{
			ints: []int{1},
		},
		{
			ints: []int{1, 2},
			perm: true,
		},
		{
			ints: []int{1, 2, 3},
			perm: true,
		},
		{
			ints: []int{1, 2, 3, 4},
			perm: true,
		},
		{
			ints: []int{1, 2, 3, 4, 5},
			perm: true,
		},
	} {
		exp := append(([]int)(nil), test.ints...)
		sort.Ints(exp)

		var xss [][]int
		if test.perm {
			xss = perm(test.ints)
		} else {
			xss = [][]int{test.ints}
		}
		for _, xs := range xss {
			name := fmt.Sprintf("%v", xs)
			t.Run(name, func(t *testing.T) {
				l := buildList(xs...)
				Sort(l, func(a, b interface{}) bool {
					return a.(int) < b.(int)
				})
				act := listInts(l)
				if !cmp.Equal(act, exp) {
					t.Fatalf("unexpected Sort() result:\n%s", cmp.Diff(exp, act))
				}
			})
		}
	}
}

func perm(xs []int) [][]int {
	var f func(int, []int) [][]int
	f = func(head int, tail []int) (ret [][]int) {
		if len(tail) == 0 {
			return [][]int{{head}}
		}
		for _, xs := range f(tail[0], tail[1:]) {
			h := len(xs)
			xs = append(xs, head)
			ret = append(ret, xs) // one with head at highest index.
			for i := 0; i < h; i++ {
				cp := append(([]int)(nil), xs...)
				cp[i], cp[h] = cp[h], cp[i]
				ret = append(ret, cp)
			}
		}
		return ret
	}
	return f(xs[0], xs[1:])
}

func buildList(xs ...int) *list.List {
	l := list.New()
	for _, x := range xs {
		l.PushBack(x)
	}
	return l
}

func listInts(l *list.List) []int {
	ret := make([]int, 0, l.Len())
	for el := l.Front(); el != nil; el = el.Next() {
		ret = append(ret, el.Value.(int))
	}
	return ret
}

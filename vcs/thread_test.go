package vcs

import (
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type CommentString string

func (s CommentString) Line() int            { return 0 }
func (s CommentString) Body() string         { return string(s) }
func (s CommentString) CreatedAt() time.Time { return time.Time{} }
func (s CommentString) UpdatedAt() time.Time { return time.Time{} }
func (s CommentString) UserLogin() string    { return "test" }
func (s CommentString) Parent() Comment      { return nil }

func (s CommentString) Compare(c Comment) int {
	return strings.Compare(
		string(s),
		string(c.(CommentString)),
	)
}

func TestSortThread(t *testing.T) {
	for _, test := range [][]string{
		[]string{
			// empty
		},
		[]string{
			"c",
		},
		[]string{
			"c", "a",
		},
		[]string{
			"c", "a", "b",
		},
		[]string{
			"c", "a", "b", "d",
		},
		[]string{
			"a", "b", "c", "d",
		},
		[]string{
			"c", "a", "b", "d", "e",
		},
		[]string{
			"c", "a", "b", "d", "e", "f",
		},
	} {
		t.Run(strconv.Itoa(len(test)), func(t *testing.T) {
			head := buildList(test...)

			head = mergeSort(head)
			act := listStrings(head)

			exp := append([]string{}, test...)
			sort.Strings(exp)

			if !cmp.Equal(exp, act) {
				t.Fatalf("unexpected sort result:\n%s", cmp.Diff(exp, act))
			}
		})
	}
}

func TestSplitThread(t *testing.T) {
	for _, test := range [][]string{
		[]string{
			"c",
		},
		[]string{
			"c", "a",
		},
		[]string{
			"c", "a", "b",
		},
		[]string{
			"c", "a", "b", "d",
		},
		[]string{
			"c", "a", "b", "d", "e",
		},
		[]string{
			"c", "a", "b", "d", "e", "f",
		},
	} {
		t.Run(strconv.Itoa(len(test)), func(t *testing.T) {
			head := buildList(test...)
			m := (len(test) + 1) / 2
			lo, hi := split(head)
			if act, exp := listStrings(lo), test[:m]; !cmp.Equal(exp, act) {
				t.Fatalf("unexpected lo half:\n%s", cmp.Diff(exp, act))
			}
			if act, exp := listStrings(hi), test[m:]; !cmp.Equal(exp, act) {
				t.Fatalf("unexpected hi half:\n%s", cmp.Diff(exp, act))
			}
		})
	}
}

func buildList(cs ...string) (head *node) {
	var tail *node
	for _, s := range cs {
		n := &node{
			comment: CommentString(s),
		}
		if head == nil {
			head = n
			tail = n
		} else {
			tail.next = n
			tail = n
		}
	}
	return head
}

func listStrings(head *node) (ret []string) {
	ret = []string{}
	for el := head; el != nil; el = el.next {
		ret = append(ret, string(el.comment.(CommentString)))
	}
	return ret
}

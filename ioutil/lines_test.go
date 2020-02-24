package ioutil

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLineSeeker(t *testing.T) {
	for _, test := range []struct {
		name  string
		lines []string
	}{
		{
			lines: []string{
				"foo",
				"bar",
				"baz",
			},
		},
	} {
		for i := range test.lines {
			t.Run(fmt.Sprintf("%s/%d", test.name, i), func(t *testing.T) {
				src := strings.NewReader(strings.Join(test.lines, "\n"))
				s := LineSeeker{
					Source: src,
				}
				if err := s.SeekLine(i); err != nil {
					t.Fatal(err)
				}
				act, err := s.ReadLine()
				if err != nil {
					t.Fatal(err)
				}
				exp := []byte(test.lines[i])
				if !bytes.Equal(act, exp) {
					t.Fatalf("unexpected #%d line contents:\n%s", i, cmp.Diff(exp, act))
				}
			})
		}
	}
}

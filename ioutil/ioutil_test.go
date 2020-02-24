package ioutil

import (
	"bytes"
	"io"
	"testing"
)

func TestWrapLineWriter(t *testing.T) {
	for _, test := range []struct {
		name string
		size int
		in   []string
		exp  string
	}{
		{
			size: 80,
			in: []string{
				"foo", "bar", "baz",
			},
			exp: "foobarbaz",
		},
		{
			size: 5,
			in: []string{
				"xxx yyyyyyz zzzz",
			},
			exp: "xxx\nyyyyyyz\nzzzz",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWrapLineWriter(&buf, test.size)
			for _, s := range test.in {
				act, err := io.WriteString(w, s)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if exp := len(s); act != exp {
					t.Fatalf(
						"unexpected written bytes: %d; want %d",
						act, exp,
					)
				}
			}
			if err := w.Flush(); err != nil {
				t.Fatal(err)
			}
			if act, exp := buf.String(), test.exp; act != exp {
				t.Fatalf(
					"unexpected outcome:\n%#q\n%#q",
					act, exp,
				)
			}
		})
	}
}

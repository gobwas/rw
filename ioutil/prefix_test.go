package ioutil

import (
	"bytes"
	"io"
	"testing"
)

func TestLinePrefixWriter(t *testing.T) {
	for _, test := range []struct {
		name   string
		prefix string
		in     []string
		exp    string
	}{
		{
			prefix: "----",
			in: []string{
				"foo\nbar", "baz", "\n",
			},
			exp: "----foo\n----barbaz\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := LinePrefixWriter{
				W:      &buf,
				Prefix: []byte(test.prefix),
			}
			for _, s := range test.in {
				act, err := io.WriteString(&w, s)
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
			if act, exp := buf.String(), test.exp; act != exp {
				t.Fatalf(
					"unexpected outcome:\n%#q\n%#q",
					act, exp,
				)
			}
		})
	}
}

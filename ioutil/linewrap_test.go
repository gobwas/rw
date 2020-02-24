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
		pad  byte
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
		{
			size: 5,
			in: []string{
				"xxx\nyyyyyy",
			},
			exp: "xxx\nyyyyyy",
		},
		{
			size: 5,
			pad:  '-',
			in: []string{
				"xxx\nyyyyyy",
			},
			exp: "xxx--\nyyyyyy",
		},
		{
			size: 5,
			pad:  '-',
			in: []string{
				"xxxxx\nyyyyyy",
			},
			exp: "xxxxx\nyyyyyy",
		},
		{
			size: 5,
			pad:  '-',
			in: []string{
				"xxxxx",
				"\nyyyyyy",
			},
			exp: "xxxxx\nyyyyyy",
		},
		{
			size: 5,
			pad:  '-',
			in: []string{
				"xxxxx",
				"\n",
				"yyyyyy",
			},
			exp: "xxxxx\nyyyyyy",
		},
		{
			name: "unicode",
			size: 7,
			pad:  '*',
			in: []string{
				"кошка",
				"\n",
				"пёсик",
			},
			exp: "кошка**\nпёсик",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewLineWrapWriter(&buf, test.size)
			w.SetPad(test.pad)

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
					"unexpected outcome:\nact: %#q\nexp: %#q",
					act, exp,
				)
			}
		})
	}
}

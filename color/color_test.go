package color

import (
	"bytes"
	"testing"
)

func TestFilter(t *testing.T) {
	for _, test := range []struct {
		name string
		in   []byte
		exp  []byte
	}{
		{
			in:  []byte(Sprint(Grey, "hello")),
			exp: []byte("hello"),
		},
	} {
		t.Run(test.name+"/bytes", func(t *testing.T) {
			act := Filter(test.in)
			exp := test.exp
			if !bytes.Equal(exp, act) {
				t.Fatalf(
					"unexpected Filter(%#q): %#q; want %#q",
					test.in, act, exp,
				)
			}
		})
		t.Run(test.name+"/string", func(t *testing.T) {
			act := FilterString(string(test.in))
			exp := string(test.exp)
			if exp != act {
				t.Fatalf(
					"unexpected FilterString(%#q): %#q; want %#q",
					test.in, act, exp,
				)
			}
		})
	}
}

package ioutil

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTrimLinesLeft(t *testing.T) {
	for _, test := range []struct {
		name string
		n    int
		bts  []byte
		exp  []byte
	}{
		{
			n: 0,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: lines(
				"foo",
				"bar",
				"baz",
			),
		},
		{
			n: 1,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: lines(
				"bar",
				"baz",
			),
		},
		{
			n: 2,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: lines(
				"baz",
			),
		},
		{
			n: 3,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: nil,
		},
		{
			n: 4,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: nil,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			act := TrimLinesLeft(test.bts, test.n)
			if exp := test.exp; !bytes.Equal(exp, act) {
				t.Fatalf("unexpected trim result:\n%s", cmp.Diff(exp, act))
			}
		})
	}
}

func TestTrimLinesRight(t *testing.T) {
	for _, test := range []struct {
		name string
		n    int
		bts  []byte
		exp  []byte
	}{
		{
			n: 0,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: lines(
				"foo",
				"bar",
				"baz",
			),
		},
		{
			n: 1,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: lines(
				"foo",
				"bar",
			),
		},
		{
			n: 2,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: lines(
				"foo",
			),
		},
		{
			n: 3,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: nil,
		},
		{
			n: 4,
			bts: lines(
				"foo",
				"bar",
				"baz",
			),
			exp: nil,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			act := TrimLinesRight(test.bts, test.n)
			if exp := test.exp; !bytes.Equal(exp, act) {
				t.Fatalf("unexpected trim result:\n%s", cmp.Diff(exp, act))
			}
		})
	}
}

func lines(ss ...string) (ret []byte) {
	for _, s := range ss {
		ret = append(ret, s...)
		ret = append(ret, '\n')
	}
	return ret
}

package ed

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseCommand(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		exp   Command
		err   bool
	}{
		{
			input: "1a",
			exp: Command{
				Start: 1,
				End:   1,
				Mode:  Add,
			},
		},
		{
			input: "1,2c",
			exp: Command{
				Start: 1,
				End:   2,
				Mode:  Change,
			},
		},
		{
			input: "1,2,3a",
			err:   true,
		},
		{
			input: "1",
			err:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			act, err := ParseCommand([]byte(test.input))
			if test.err && err == nil {
				t.Fatalf("want error; got nil")
			}
			if !test.err && err != nil {
				t.Fatalf("unexpected error; %v", err)
			}
			if test.err {
				return
			}
			if exp := test.exp; act != exp {
				t.Fatalf("unexpected command:\n%s", cmp.Diff(act, exp))
			}
		})
	}
}

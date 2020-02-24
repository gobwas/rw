package ed

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDiff(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		exp   []Command
		err   bool
	}{
		{
			input: strings.TrimSpace(`
60a
// Whut?
.
			`),
			exp: []Command{
				{
					Start: 60,
					End:   60,
					Mode:  ModeAdd,
					Text:  []byte("// Whut?\n"),
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var act []Command
			err := Diff(strings.NewReader(test.input), func(cmd Command) {
				act = append(act, cmd)
			})
			if test.err {
				if err == nil {
					t.Fatalf("want error; got nil")
				}
				return
			}
			if !test.err && err != nil {
				t.Fatalf("unexpected error; %v", err)
			}
			if exp := test.exp; !cmp.Equal(exp, act) {
				t.Fatalf("unexpected command(s):\n%s", cmp.Diff(exp, act))
			}
		})
	}
}

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
				Mode:  ModeAdd,
			},
		},
		{
			input: "1,2c",
			exp: Command{
				Start: 1,
				End:   2,
				Mode:  ModeChange,
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
			if test.err {
				if err == nil {
					t.Fatalf("want error; got nil")
				}
				return
			}
			if !test.err && err != nil {
				t.Fatalf("unexpected error; %v", err)
			}
			if exp := test.exp; !cmp.Equal(exp, act) {
				t.Fatalf("unexpected command:\n%s", cmp.Diff(exp, act))
			}
		})
	}
}

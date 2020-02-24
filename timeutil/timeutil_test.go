package timeutil

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	for _, test := range []struct {
		name string
		d    time.Duration
		exp  string
	}{
		{
			name: "zero",
			exp:  "0s",
		},
		{
			d:   time.Second,
			exp: "1s",
		},
		{
			d:   time.Minute,
			exp: "1m",
		},
		{
			d:   time.Minute + time.Second,
			exp: "1m1s",
		},
		{
			d:   2*time.Minute + time.Second,
			exp: "2m",
		},
		{
			d:   time.Hour,
			exp: "1h",
		},
		{
			d:   time.Hour + time.Minute,
			exp: "1h1m",
		},
		{
			d:   time.Hour + time.Minute + time.Second,
			exp: "1h1m",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			act := FormatDuration(test.d)
			if exp := test.exp; act != exp {
				t.Fatalf(
					"unexpected format of %s: %q; want %q",
					test.d, act, exp,
				)
			}
		})
	}
}

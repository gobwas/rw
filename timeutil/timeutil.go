package timeutil

import (
	"strconv"
	"strings"
	"time"
)

type timeUnit struct {
	name  string
	short byte
	size  time.Duration
}

var timeUnits = []timeUnit{
	{
		name:  "year",
		short: 'y',
		size:  365 * 24 * time.Hour,
	},
	{
		name:  "week",
		short: 'w',
		size:  7 * 24 * time.Hour,
	},
	{
		name:  "day",
		short: 'd',
		size:  24 * time.Hour,
	},
	{
		name:  "hour",
		short: 'h',
		size:  time.Hour,
	},
	{
		name:  "minute",
		short: 'm',
		size:  time.Minute,
	},
	{
		name:  "second",
		short: 's',
		size:  time.Second,
	},
}

func formatDuration(d time.Duration, fn func(int, timeUnit, int64) bool) {
	for i, u := range timeUnits {
		n := d / u.size
		if n == 0 {
			continue
		}
		d -= n * u.size

		if !fn(i, u, int64(n)) {
			return
		}
	}
}

var zeroTimeUnit timeUnit

func FormatDuration(d time.Duration) string {
	var sb strings.Builder
	var (
		highUnit  int = -1
		highValue int64
	)
	formatDuration(d, func(i int, u timeUnit, n int64) bool {
		if highUnit != -1 && (i-highUnit > 1 || highValue > 1) {
			return false
		}
		if highUnit == -1 {
			highUnit = i
			highValue = n
		}
		sb.WriteString(strconv.FormatInt(n, 10))
		sb.WriteByte(u.short)
		return true
	})
	if sb.Len() == 0 {
		sb.WriteByte('0')
		sb.WriteByte(timeUnits[len(timeUnits)-1].short)
	}
	return sb.String()
}

func FormatSince(t time.Time) string {
	d := time.Since(t)
	var sb strings.Builder
	formatDuration(d, func(_ int, u timeUnit, n int64) bool {
		if n > 1 {
			sb.WriteString(strconv.FormatInt(n, 10))
			sb.WriteString(" ")
		}
		sb.WriteString(u.name)
		sb.WriteString(plural(n))
		return false
	})
	if sb.Len() == 0 {
		return "now"
	}
	return sb.String()
}

func plural(n int64) string {
	if n <= 1 {
		return ""
	}
	return "s"
}

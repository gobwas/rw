package color

import (
	"fmt"
	"io"
	"strings"
)

var ()

type Color uint

const (
	Unknown Color = iota
	Black
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
)

var colors = [...][]byte{
	0: []byte("\u001b[0m"),
	//Grey:    []byte("\u001b[38;5;245m"),
	Black:   []byte("\u001b[30;1m"),
	Red:     []byte("\u001b[31;1m"),
	Green:   []byte("\u001b[32;2m"),
	Yellow:  []byte("\u001b[33;1m"),
	Blue:    []byte("\u001b[34;1m"),
	Magenta: []byte("\u001b[35;1m"),
	Cyan:    []byte("\u001b[35;1m"),
	White:   []byte("\u001b[37;1m"),
}

func Fbegin(w io.Writer, color Color) (int, error) {
	return w.Write(colors[color])
}
func Freset(w io.Writer) (int, error) {
	return w.Write(colors[0])
}

func Sprintf(color Color, format string, args ...interface{}) string {
	var sb strings.Builder
	Fbegin(&sb, color)
	fmt.Fprintf(&sb, format, args...)
	Freset(&sb)
	return sb.String()
}

func Fprintf(w io.Writer, color Color, format string, args ...interface{}) (n int, err error) {
	var m int
	m, err = Fbegin(w, color)
	if err != nil {
		return n, err
	}
	n += m

	m, err = fmt.Fprintf(w, format, args...)
	if err != nil {
		return n, err
	}
	n += m

	m, err = Freset(w)
	if err != nil {
		return n, err
	}
	n += m

	return n, nil
}

package color

import (
	"bytes"
	"fmt"
	"io"
	"os"
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
	Grey
)

func Filter(p []byte) (buf []byte) {
	for len(p) > 0 {
		i := bytes.Index(p, []byte("\u001b"))
		if i == -1 {
			buf = append(buf, p...)
			break
		}
		j := bytes.IndexByte(p[i:], 'm')
		if j == -1 {
			buf = append(buf, p...)
			break
		}
		buf = append(buf, p[:i]...)
		p = p[i+j+1:]
	}
	return buf
}

func FilterString(s string) string {
	var sb strings.Builder
	for len(s) > 0 {
		i := strings.Index(s, "\u001b")
		if i == -1 {
			sb.WriteString(s)
			break
		}
		j := strings.IndexByte(s[i:], 'm')
		if j == -1 {
			sb.WriteString(s)
			break
		}
		sb.WriteString(s[:i])
		s = s[i+j+1:]
	}
	return sb.String()
}

var colors = [...][]byte{
	0:       []byte("\u001b[0m"),
	Grey:    []byte("\u001b[38;5;245m"),
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

func Sprint(color Color, args ...interface{}) string {
	var sb strings.Builder
	Fbegin(&sb, color)
	fmt.Fprint(&sb, args...)
	Freset(&sb)
	return sb.String()
}

func Sprintf(color Color, format string, args ...interface{}) string {
	var sb strings.Builder
	Fbegin(&sb, color)
	fmt.Fprintf(&sb, format, args...)
	Freset(&sb)
	return sb.String()
}

func Fprintln(w io.Writer, color Color, s string) (int, error) {
	return write(w, color, func() (int, error) {
		return fmt.Fprintln(w, s)
	})
}

func Fprintf(w io.Writer, color Color, format string, args ...interface{}) (int, error) {
	return write(w, color, func() (int, error) {
		return fmt.Fprintf(w, format, args...)
	})
}

func Fwrite(w io.Writer, color Color, p []byte) (int, error) {
	return write(w, color, func() (int, error) {
		return w.Write(p)
	})
}

func Println(color Color, s string) (int, error) {
	return Fprintln(os.Stdout, color, s)
}

func Printf(color Color, format string, args ...interface{}) (int, error) {
	return Fprintf(os.Stdout, color, format, args...)
}

func Write(color Color, p []byte) (int, error) {
	return Fwrite(os.Stdout, color, p)
}

func write(w io.Writer, color Color, fn func() (int, error)) (n int, err error) {
	var m int
	m, err = Fbegin(w, color)
	if err != nil {
		return n, err
	}
	n += m

	m, err = fn()
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

package ioutil

import (
	"bytes"
	"io"
	"unicode/utf8"
)

// LineWrapWriter is an io.Writer which writes up to N characters per each
// line. It supports UTF8 encoding.
type LineWrapWriter struct {
	w io.Writer

	err   error
	buf   bytes.Buffer
	lim   int
	runes int
	pad   byte

	runeCounter func([]byte) int
}

func NewLineWrapWriter(w io.Writer, limit int) *LineWrapWriter {
	return &LineWrapWriter{
		w:   w,
		lim: limit,
	}
}

func (w *LineWrapWriter) SetPad(c byte) {
	w.pad = c
}

func (w *LineWrapWriter) SetRuneCounter(fn func([]byte) int) {
	w.runeCounter = fn
}

func (w *LineWrapWriter) runeCount(p []byte) int {
	if fn := w.runeCounter; fn != nil {
		return fn(p)
	}
	return utf8.RuneCount(p)
}

var newline = []byte{'\n'}

func (w *LineWrapWriter) Flush() error {
	if w.err != nil {
		return w.err
	}
	_, w.err = w.buf.WriteTo(w.w)
	return w.err
}

func (w *LineWrapWriter) Write(p []byte) (n int, err error) {
	for w.err == nil && len(p) > 0 {
		var (
			noLine   bool
			overflow bool
		)
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			i = len(p)
			noLine = true
		}
		c := w.runeCount(p[:i])
		for w.runes+c > w.lim {
			j := bytes.LastIndexByte(p[:i], ' ') // TODO any space actually \t, etc.
			if j == -1 {
				overflow = true
				break
			}
			c -= i - j
			i = j
			noLine = false
		}

		if noLine && !overflow {
			var m int
			m, w.err = w.buf.Write(p[:i])
			n += m
			w.runes += c
			break
		}

		if padChar := w.pad; overflow || padChar == 0 {
			_, w.err = w.buf.WriteTo(w.w)
			if w.err == nil {
				var m int
				m, w.err = w.w.Write(p[:i])
				n += m
			}
			if w.err == nil && !noLine {
				_, w.err = w.w.Write(newline)
				n += 1
			}
		} else {
			_, w.err = w.buf.WriteTo(w.w)
			if w.err == nil {
				var m int
				m, w.err = w.w.Write(p[:i])
				n += m
			}
			if w.err == nil {
				_, w.err = w.w.Write(bytes.Repeat([]byte{padChar}, w.lim-w.runes-c))
			}
			if w.err == nil {
				_, w.err = w.w.Write(newline)
				n += 1
			}
		}
		w.runes = 0
		if i == len(p) {
			break
		}
		p = p[i+1:]
	}
	return n, w.err
}

func (w *LineWrapWriter) TODOWriteString(s string) (int, error) {
	return -1, nil
}
func (w *LineWrapWriter) TODOReadFrom(src io.Reader) (int64, error) {
	return -1, nil
	//p := make([]byte, n)
	//var (
	//	firstSpace bool
	//)
	//for {
	//	m, err := src.Read(p)
	//	if m == 0 && err != nil {
	//		return err
	//	}
	//	if m < n {
	//		dst.Write(p[:m])
	//		return nil
	//	}
	//	var i int
	//	if firstSpace {
	//		i = bytes.IndexByte(p, ' ')
	//	} else {
	//		i = bytes.LastIndexByte(p, ' ')
	//	}
	//	if i == -1 {
	//		dst.Write(p)
	//		firstSpace = true
	//		continue
	//	}
	//	firstSpace = false
	//	dst.Write(p[:i])
	//	dst.Write([]byte{'\n'})
	//	dst.Write(p[i+1:])
	//}
	//return nil
}

func (w *LineWrapWriter) Reset(dest io.Writer) {
	w.w = dest
	w.err = nil
}

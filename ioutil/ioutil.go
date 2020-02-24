package ioutil

import (
	"bytes"
	"io"
)

type LineWrapWriter struct {
	w io.Writer

	err      error
	buf      []byte
	pos      int
	overflow bool
}

func NewLineWrapWriter(w io.Writer, limit int) *LineWrapWriter {
	return &LineWrapWriter{
		w:   w,
		buf: make([]byte, limit),
	}
}

var newline = []byte{'\n'}

func (w *LineWrapWriter) Flush() error {
	if w.err != nil {
		return w.err
	}
	w.flush()
	return w.err
}

func (w *LineWrapWriter) Write(p []byte) (n int, err error) {
	for w.err == nil && len(p) > 0 {
		m := copy(w.buf[w.pos:], p)
		w.pos += m
		n += m
		p = p[m:]
		if len(p) == 0 {
			return n, nil
		}

		var i int
		if w.overflow {
			i = bytes.IndexByte(w.buf[:w.pos], ' ')
		} else {
			i = bytes.LastIndexByte(w.buf[:w.pos], ' ')
		}
		if i == -1 {
			w.overflow = true
			w.flush()
		} else {
			w.overflow = false
			w.buf[i] = '\n'
			_, w.err = w.w.Write(w.buf[:i+1])
			w.pos = copy(w.buf, w.buf[i+1:])
		}
	}
	return n, w.err
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
	w.pos = 0
	w.overflow = false
}

func (w *LineWrapWriter) flush() error {
	_, w.err = w.w.Write(w.buf[:w.pos])
	w.pos = 0
	return w.err
}

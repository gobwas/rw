package ioutil

import (
	"bytes"
	"io"
)

type LinePrefixWriter struct {
	W      io.Writer
	Prefix []byte

	dirty bool
}

func NewLinePrefixWriter(w io.Writer, prefix []byte) *LinePrefixWriter {
	return &LinePrefixWriter{
		W:      w,
		Prefix: prefix,
	}
}

func (w *LinePrefixWriter) Write(p []byte) (n int, err error) {
	var m int
	for len(p) > 0 {
		if !w.dirty {
			if _, err = w.W.Write(w.Prefix); err != nil {
				return 0, err
			}
			w.dirty = true
		}
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			m, err = w.W.Write(p)
			return n + m, err
		}
		m, err = w.W.Write(p[:i+1])
		n += m
		if err != nil {
			return n, err
		}
		p = p[i+1:]
		w.dirty = false
	}
	return n, nil
}

type LineSuffixWriter struct {
	W      io.Writer
	Suffix []byte
}

func NewLineSuffixWriter(w io.Writer, suffix []byte) *LineSuffixWriter {
	return &LineSuffixWriter{
		W:      w,
		Suffix: suffix,
	}
}

func (w *LineSuffixWriter) Write(p []byte) (n int, err error) {
	var m int
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			m, err = w.W.Write(p)
			return n + m, err
		}
		m, err = w.W.Write(p[:i])
		n += m
		if err != nil {
			return n, err
		}
		_, err = w.W.Write(w.Suffix)
		if err != nil {
			return n, err
		}
		_, err = w.W.Write([]byte{'\n'})
		if err != nil {
			return n, err
		}
		n += 1
		p = p[i+1:]
	}
	return n, nil
}

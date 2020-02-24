package ioutil

import (
	"bytes"
	"io"
)

type LinePrefixWriter struct {
	W      io.Writer
	Prefix []byte
}

func (w *LinePrefixWriter) Write(p []byte) (n int, err error) {
	var m int
	for len(p) > 0 {
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
		_, err = w.W.Write(w.Prefix)
		if err != nil {
			return n, err
		}
		p = p[i+1:]
	}
	return n, nil
}

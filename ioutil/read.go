package ioutil

import (
	"bufio"
)

// ReadLine either returns a non-nil line or it returns an error, never both.
func ReadLine(r *bufio.Reader, buf []byte) (line, _ []byte, err error) {
	for {
		line, isPrefix, err := r.ReadLine()
		if err != nil {
			return nil, buf, err
		}
		if isPrefix || len(buf) > 0 {
			buf = append(buf, line...)
			line = buf
		}
		if !isPrefix {
			return line, buf, nil
		}
	}
}

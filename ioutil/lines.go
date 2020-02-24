package ioutil

import (
	"bytes"
)

func TrimLinesLeft(buf []byte, n int) []byte {
	for m := 0; len(buf) > 0 && m < n; m++ {
		i := bytes.IndexByte(buf, '\n')
		if i == -1 {
			return nil
		}
		buf = buf[i+1:]
	}
	return buf
}

func TrimLinesRight(buf []byte, n int) []byte {
	for m := 0; len(buf) > 0 && m < n; m++ {
		var (
			size = len(buf)
			skip = 0
		)
		if size > 0 && buf[size-1] == '\n' {
			skip = 1
		}
		i := bytes.LastIndexByte(buf[:size-skip], '\n')
		if i == -1 {
			return nil
		}
		buf = buf[:i+1]
	}
	return buf
}

func LineSliceOld(buf *bytes.Buffer, first, last int) *bytes.Buffer {
	bts := buf.Bytes()
	bts = TrimLinesLeft(bts, first)
	bts = TrimLinesRight(bts, last)
	b := bytes.NewBuffer(nil)
	b.Write(bts)
	return b

	lines := bytes.Split(buf.Bytes(), []byte{'\n'})
	ret := new(bytes.Buffer)
	for _, line := range lines[first : len(lines)-last-1] {
		ret.Write(line)
		ret.WriteByte('\n')
	}
	return ret
}

func LineSlice(bts []byte, lo, hi int) []byte {
	return bts
}

func SplitFirst(p []byte, c byte) (head, tail []byte) {
	i := bytes.IndexByte(p, c)
	if i == -1 {
		return p, nil
	}
	return p[:i], p[i+1:]
}

func SplitLast(p []byte, c byte) (head, tail []byte) {
	i := bytes.LastIndexByte(p, c)
	if i == -1 {
		return nil, p
	}
	return p[:i], p[i+1:]
}

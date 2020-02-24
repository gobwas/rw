package ioutil

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/gobwas/avl"
)

type LineSeeker struct {
	Source io.ReadSeeker

	buf   []byte
	br    *bufio.Reader
	index avl.Tree
}

func SeekLines(r io.ReadSeeker) *LineSeeker {
	return &LineSeeker{
		Source: r,
	}
}

func (s *LineSeeker) SeekLine(i int) (err error) {
	if i <= 0 {
		return s.seek(0)
	}
	p := s.search(i)
	if p == nil {
		p, err = s.scroll(i)
		if err != nil {
			return err
		}
	}
	return s.seek(p.offset)
}

func (s *LineSeeker) Read(p []byte) (int, error) {
	return s.br.Read(p)
}

// ReadLine either returns a non-nil line or it returns an error, never both.
func (s *LineSeeker) ReadLine() (line []byte, err error) {
	line, s.buf, err = ReadLine(s.br, s.buf[:0])
	return line, err
}

func (s *LineSeeker) seek(offset int64) error {
	_, err := s.Source.Seek(offset, io.SeekStart)
	if err != nil {
		return err
	}
	if s.br != nil {
		s.br.Reset(s.Source)
	} else {
		s.br = bufio.NewReader(s.Source)
	}
	return nil
}

func (s *LineSeeker) search(i int) *position {
	p := s.index.Search(searchLine(i))
	if p == nil {
		return nil
	}
	return p.(*position)
}

func (s *LineSeeker) scroll(i int) (p *position, err error) {
	var (
		line   int
		offset int64
	)
	x := s.index.Predecessor(searchLine(i))
	if x != nil {
		// Some lines were indexed before.
		p := x.(*position)
		line = p.line
		offset = p.offset
	}
	if err := s.seek(offset); err != nil {
		return nil, err
	}
	for line < i {
		var bts []byte
		bts, s.buf, err = ReadLine(s.br, s.buf[:0])
		if err != nil {
			return nil, err
		}

		offset += int64(len(bts)) + 1 // +1 is for \n.
		line++

		p = &position{
			line:   line,
			offset: offset,
		}
		var existing avl.Item
		s.index, existing = s.index.Insert(p)
		if existing != nil {
			panic("inconsistent state")
		}
	}
	return p, nil
}

type searchLine int

func (s searchLine) Compare(x avl.Item) int {
	a := int(s)
	b := x.(*position).line
	return compare(a, b)
}

type position struct {
	line   int
	offset int64
}

func (p *position) Compare(x avl.Item) int {
	a := p.line
	b := x.(*position).line
	return compare(a, b)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func compare(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Not counting '\n'.
func MaxLineRunesInString(s string) (max int) {
	max = -1
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i == -1 {
			if max == -1 {
				return utf8.RuneCountInString(s)
			}
			return max
		}
		n := utf8.RuneCountInString(s[:i])
		if n > max {
			max = n
		}
		s = s[i+1:]
	}
	return max
}

func MaxLineRunes(p []byte) (max int) {
	max = -1
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			if max == -1 {
				return utf8.RuneCount(p)
			}
			return max
		}
		n := utf8.RuneCount(p[:i])
		if n > max {
			max = n
		}
		p = p[i+1:]
	}
	return max
}

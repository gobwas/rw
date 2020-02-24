package rw

import (
	"bytes"
	"container/list"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"

	"github.com/gobwas/rw/color"
	"github.com/gobwas/rw/ed"
	"github.com/gobwas/rw/ioutil"
)

type quick struct {
	base *ioutil.LineSeeker

	// This can be list of edits.
	edits list.List // List<*editBuffer>

	baseStart int
	baseLine  int
	baseLines int

	headOffset int
	headStart  int
	headLine   int
	headLines  int
}

func newQuick(base *os.File) *quick {
	return &quick{
		base: &ioutil.LineSeeker{Source: base},
	}
}

type buffer []byte

func (b buffer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte(b))
	return int64(n), err
}

type editBuffer struct {
	el *list.Element

	cmd ed.Command
	bts []byte

	baseStart int
	baseLines int
	headStart int
	headLines int
}

func (e *editBuffer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(e.bts)
	return int64(n), err
}

func (e *editBuffer) baseStop() int {
	return e.baseStart + e.baseLines
}

func (e *editBuffer) headStop() int {
	return e.headStart + e.headLines
}

func baseDistance(prev, next *editBuffer) int {
	return next.baseStart - prev.baseStop()
}

func isConsecutive(prev, next *editBuffer) bool {
	return baseDistance(prev, next) == 0
}

func joinBuffers(prev, next *editBuffer) *editBuffer {
	if !isConsecutive(prev, next) {
		panic("buffers are not consecutive")
	}
	var buf bytes.Buffer
	prev.WriteTo(&buf)
	next.WriteTo(&buf)
	return &editBuffer{
		bts:       append(append(([]byte)(nil), prev.bts...), next.bts...),
		baseStart: prev.baseStart,
		baseLines: prev.baseLines + next.baseLines,
		headStart: prev.headStart,
		headLines: prev.headLines + next.headLines,
	}
}

func (q *quick) Render(edits []ed.Command) {
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Start < edits[j].Start
	})
	var buf bytes.Buffer
	for _, cmd := range edits {
		log.Printf(
			"processing out command: %s: %d,%d: %#q",
			cmd.Mode, cmd.Start, cmd.End, cmd.Text,
		)

		q.seek(cmd.Start)

		switch cmd.Mode {
		case ed.ModeAdd:
			// NOTE: lines are added always after cmd.Start.
			// So need to flush one more line.
			q.advanceHead(1)
			q.insertLines(&buf, cmd)

		case ed.ModeChange:
			for q.baseLine <= cmd.End {
				q.deleteLine(&buf)
			}
			q.insertLines(&buf, cmd)

		case ed.ModeDelete:
			// NOTE: lines deleted are in inclusive range [cmd.Start, cmd.End].
			for q.baseLine <= cmd.End {
				q.deleteLine(&buf)
			}
		}

		e := &editBuffer{
			cmd:       cmd,
			bts:       append(([]byte)(nil), buf.Bytes()...),
			baseStart: q.baseStart,
			baseLines: q.baseLines,
			headStart: q.headStart,
			headLines: q.headLines,
		}
		e.el = q.edits.PushBack(e)

		buf.Reset()
	}
}

func (q *quick) Front() *editBuffer {
	return bufferFromElement(q.edits.Front())
}

func (q *quick) Prev(e *editBuffer) *editBuffer {
	return bufferFromElement(e.el.Prev())
}

func (q *quick) Next(e *editBuffer) *editBuffer {
	return bufferFromElement(e.el.Next())
}

func bufferFromElement(el *list.Element) *editBuffer {
	if el == nil {
		return nil
	}
	return el.Value.(*editBuffer)
}

func (q *quick) Buffers(it func(*editBuffer)) {
	for el := q.edits.Front(); el != nil; el = el.Next() {
		it(el.Value.(*editBuffer))
	}
}

func (q *quick) seek(start int) {
	q.seekBase(start)

	q.baseStart = start
	q.baseLine = q.baseStart
	q.baseLines = 0

	q.headStart = q.baseStart + q.headOffset
	q.headLine = q.headStart
	q.headLines = 0
}

func (q *quick) seekBase(line int) {
	// NOTE: line seeker indexes line numbers from 0, that's why here is "-1".
	if err := q.base.SeekLine(line - 1); err != nil {
		panic(fmt.Sprintf(
			"quick: unexpected seek line #%d error: %v",
			line, err,
		))
	}
}

func (q *quick) printHeadLine(w io.Writer, line []byte) {
	color.Fprintf(w, color.Green,
		"+       % 4d %s\n",
		q.headLine,
		bytes.TrimRight(line, "\n"),
	)
}
func (q *quick) printBaseLine(w io.Writer, line []byte) {
	color.Fprintf(w, color.Red,
		"- % 4d       %s\n",
		q.baseLine,
		bytes.TrimRight(line, "\n"),
	)
}

func (q *quick) ExpandBetween(w io.Writer, prev, next *editBuffer) (n int) {
	lo := prev.baseStop()
	hi := next.baseStart
	return q.expand(w, lo, hi, prev.headStart-prev.baseStart)
}

func (q *quick) ExpandBefore(w io.Writer, e *editBuffer, lines int) (n int) {
	lo := e.baseStart - lines
	hi := e.baseStart
	if lo < 0 {
		lo = 0
	}
	if prev := q.Prev(e); prev != nil {
		if stop := prev.baseStop(); stop > lo {
			lo = stop
		}
	}
	return q.expand(w, lo, hi, e.headStart-e.baseStart)
}

func (q *quick) HasLinesBefore(e *editBuffer, expand int) bool {
	prev := q.Prev(e)
	if prev == nil {
		return e.baseStart-expand > 0
	}
	return (e.baseStart-expand)-prev.baseStop() > 0
}

func (q *quick) HasLinesAfter(e *editBuffer, expand int) bool {
	next := q.Next(e)
	if next == nil {
		return q.base.SeekLine(e.baseStop()+expand) == nil
	}
	return next.baseStart-(e.baseStop()+expand) > 0
}

func (q *quick) ExpandAfter(w io.Writer, e *editBuffer, lines int) (n int) {
	lo := e.baseStart + e.baseLines
	hi := lo + lines
	if next := q.Next(e); next != nil {
		if start := next.baseStart; start < hi {
			hi = start
		}
	}
	return q.expand(w, lo, hi, e.headStart-e.baseStart)
}

func (q *quick) expand(w io.Writer, lo, hi, headOffset int) (n int) {
	q.seekBase(lo)
	for lo < hi {
		bts, err := q.base.ReadLine()
		if err == io.EOF {
			// NOTE: ReadLine() never returns line AND non-nil error.
			break
		}
		if err != nil {
			panic(err)
		}
		printLine(w, lo, lo+headOffset, bts)
		lo++
		n++
	}
	return n
}

func (q *quick) deleteLine(w io.Writer) {
	line, err := q.base.ReadLine()
	if err != nil {
		panic(err)
	}
	q.printBaseLine(w, line)

	q.baseLine++
	q.baseLines++
	q.headOffset--
}

func (q *quick) advanceHead(n int) {
	q.headStart++
	q.headLine++
}

func (q *quick) insertLines(w io.Writer, cmd ed.Command) {
	for {
		line, err := cmd.Text.ReadBytes('\n')
		if err != nil {
			break
		}

		q.printHeadLine(w, line)

		q.headLine++
		q.headLines++
		q.headOffset++
	}
}

func printLine(w io.Writer, baseLine, headLine int, line []byte) {
	fmt.Fprintf(w,
		"  % 4d  % 4d %s\n",
		baseLine,
		headLine,
		bytes.TrimRight(line, "\n"),
	)
}

type lineType uint8

const (
	lineUnknown lineType = iota
	lineBase
	lineHead
)

type line struct {
	num int
	typ lineType
}

func parseLineRange(s string) (lo, hi line, err error) {
	type parser func() (parser, error)
	var (
		sepParser parser
		numParser parser
		typParser parser
	)
	var (
		num   int
		typ   lineType
		pos   int
		lines = [2]*line{&lo, &hi}
	)
	sepParser = func() (_ parser, err error) {
		if len(s) == 0 {
			return nil, nil
		}
		switch s[0] {
		case ':':
			s = s[1:]
			return numParser, nil
		default:
			return nil, fmt.Errorf("unexpected line number separator: %q (':' is expected)", s[0])
		}
	}
	numParser = func() (_ parser, err error) {
		var i int
		for ; i < len(s); i++ {
			c := s[i]
			if '0' <= c && c <= '9' {
				continue
			}
			break
		}
		num, err = strconv.Atoi(s[:i])
		if err != nil {
			return nil, err
		}
		*lines[pos] = line{
			num: num,
			typ: typ,
		}
		pos++
		if len(lines) == pos {
			return nil, nil
		}
		s = s[i:]
		return sepParser, nil
	}
	typParser = func() (parser, error) {
		if len(s) == 0 {
			return nil, fmt.Errorf("malformed string")
		}
		switch s[0] {
		case '-':
			typ = lineBase
		case '+':
			typ = lineHead
		default:
			return nil, fmt.Errorf(
				"unexpected line type specifier: %q (+ or - are expected)",
				s[0],
			)
		}
		s = s[1:]
		return numParser, nil
	}
	for p := typParser; p != nil; {
		p, err = p()
		if err != nil {
			return line{}, line{}, err
		}
	}
	return lo, hi, nil
}

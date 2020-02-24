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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gobwas/avl"
	"github.com/gobwas/rw/color"
	"github.com/gobwas/rw/ed"
	"github.com/gobwas/rw/ioutil"
	"github.com/gobwas/rw/listutil"
	"github.com/gobwas/rw/timeutil"
	"github.com/gobwas/rw/vcs"
)

type quick struct {
	base *ioutil.LineSeeker

	baseThreads avl.Tree
	headThreads avl.Tree
	commentIDs  map[string]uint
	commentID   uint

	buffers list.List // List<*editBuffer>

	baseEdits map[int]bool
	headEdits map[int]bool

	// TODO: move this counters to editBuffer context.
	baseStart int
	baseLine  int
	baseLines int

	headStart int
	headLine  int
	headLines int

	headOffset int
}

type threadList struct {
	list list.List // list.List<vcs.Comment>
}

func (ts *threadList) forEach(it func(vcs.Thread)) {
	if ts == nil {
		return
	}
	for el := ts.list.Front(); el != nil; el = el.Next() {
		it(el.Value.(vcs.Thread))
	}
}

func (ts *threadList) update(it func(vcs.Thread) vcs.Thread) {
	if ts == nil {
		return
	}
	for el := ts.list.Front(); el != nil; el = el.Next() {
		el.Value = it(el.Value.(vcs.Thread))
	}
}

func (ts *threadList) startLine() int {
	n, _ := ts.front().Lines()
	return n
}

func (ts *threadList) side() vcs.Side {
	return ts.front().Side()
}

func (ts *threadList) createdAt() time.Time {
	return ts.front().CreatedAt()
}

func (ts *threadList) front() vcs.Thread {
	return ts.list.Front().Value.(vcs.Thread)
}

func (ts *threadList) push(t vcs.Thread) {
	listutil.InsertInOrder(&ts.list, t, func(v0, v1 interface{}) bool {
		return compareThreadsByCreationTime(
			v0.(vcs.Thread),
			v1.(vcs.Thread),
		) < 0
	})
}

func (ts *threadList) sortBy(cmps ...func(a, b vcs.Thread) int) {
	listutil.Sort(&ts.list, func(v0, v1 interface{}) bool {
		t0 := v0.(vcs.Thread)
		t1 := v1.(vcs.Thread)
		for _, cmp := range cmps {
			if c := cmp(t0, t1); c != 0 {
				return c < 0
			}
		}
		return false
	})
}

func (ts *threadList) sort() {
	ts.sortBy(compareThreadsByCreationTime)
}

func (ts *threadList) Compare(x avl.Item) int {
	xs := x.(*threadList)
	if c := compareInt(ts.startLine(), xs.startLine()); c != 0 {
		return c
	}
	return 0
}

type threadsQuery struct {
	startLine int
}

func (q *threadsQuery) Compare(x avl.Item) int {
	t := x.(*threadList)
	if c := compareInt(q.startLine, t.startLine()); c != 0 {
		return c
	}
	return 0
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return +1
	}
	return 0
}

func compareTime(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case b.Before(a):
		return +1
	}
	return 0
}

func newQuick(base *os.File, comments []vcs.Comment) *quick {
	log.Printf("total number of comments: %d", len(comments))
	var (
		baseThreads avl.Tree
		headThreads avl.Tree
	)
	for _, t := range vcs.BuildThreads(comments) {
		var ts *avl.Tree
		switch t.Side() {
		case vcs.SideBase:
			ts = &baseThreads
		case vcs.SideHead:
			ts = &headThreads
		default:
			panic("unexpected vcs thread side")
		}
		start, _ := t.Lines()
		x := ts.Search(&threadsQuery{
			startLine: start,
		})
		if x == nil {
			v := new(threadList)
			v.push(t)
			*ts, _ = ts.Insert(v)
		} else {
			x.(*threadList).push(t)
		}
	}

	sortThreads := func(tree avl.Tree) {
		tree.InOrder(func(x avl.Item) bool {
			x.(*threadList).sort()
			return true
		})
	}
	sortThreads(baseThreads)
	sortThreads(headThreads)

	commendIDs := make(map[string]uint)
	var commentID uint

	baseItem, _ := baseThreads.Min().(*threadList)
	headItem, _ := headThreads.Min().(*threadList)
	for baseItem != nil || headItem != nil {
		var items [2]*threadList
		switch {
		case baseItem == nil:
			items[0] = headItem
			headItem, _ = headThreads.Successor(headItem).(*threadList)
		case headItem == nil:
			items[0] = baseItem
			baseItem, _ = baseThreads.Successor(baseItem).(*threadList)
		default:
			c := compareThreads(baseItem.front(), headItem.front())
			switch {
			case c < 0:
				items[0] = baseItem
				baseItem, _ = baseThreads.Successor(baseItem).(*threadList)
			case c > 0:
				items[0] = headItem
				headItem, _ = headThreads.Successor(headItem).(*threadList)
			default:
				items[0] = baseItem
				items[1] = headItem
				baseItem, _ = baseThreads.Successor(baseItem).(*threadList)
				headItem, _ = headThreads.Successor(headItem).(*threadList)
			}
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			item.forEach(func(t vcs.Thread) {
				for _, c := range t {
					log.Printf(
						"assigned comment ID %x for %s: %q",
						commentID, c.ID(), c.Body(),
					)
					commendIDs[c.ID()] = commentID
					commentID++
				}
			})
		}
	}

	return &quick{
		base:        &ioutil.LineSeeker{Source: base},
		baseEdits:   make(map[int]bool),
		headEdits:   make(map[int]bool),
		baseThreads: baseThreads,
		headThreads: headThreads,
		commentIDs:  commendIDs,
		commentID:   commentID,
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

	headOffset int
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
	edits = append(([]ed.Command)(nil), edits...)
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Start < edits[j].Start
	})
	var buf bytes.Buffer
	for _, cmd := range edits {
		log.Printf(
			"processing out command: %s: %d,%d: %#q",
			cmd.Mode, cmd.Start, cmd.End, cmd.Text,
		)

		prevOffset := q.headOffset

		q.renderCommand(&buf, cmd)

		e := &editBuffer{
			cmd:        cmd,
			bts:        append(([]byte)(nil), buf.Bytes()...),
			baseStart:  q.baseStart,
			baseLines:  q.baseLines,
			headStart:  q.headStart,
			headLines:  q.headLines,
			headOffset: prevOffset,
		}
		e.el = q.buffers.PushBack(e)

		buf.Reset()
	}
}

func (q *quick) renderCommand(buf *bytes.Buffer, cmd ed.Command) {
	q.seek(cmd.Start)

	switch cmd.Mode {
	case ed.ModeAdd:
		// NOTE: lines are added always after cmd.Start.
		// So need to flush one more line.
		q.advanceHead(1)
		q.insertLines(buf, cmd)

	case ed.ModeChange:
		for q.baseLine <= cmd.End {
			q.deleteLine(buf)
		}
		q.insertLines(buf, cmd)

	case ed.ModeDelete:
		// NOTE: lines deleted are in inclusive range [cmd.Start, cmd.End].
		for q.baseLine <= cmd.End {
			q.deleteLine(buf)
		}
	}
}

func (q *quick) RenderBuffer(e *editBuffer) {
	var buf bytes.Buffer
	q.headOffset = e.headOffset
	q.renderCommand(&buf, e.cmd)
	e.bts = buf.Bytes()
}

func (q *quick) AppendComment(c vcs.Comment) {
	var ts *avl.Tree
	switch c.Side() {
	case vcs.SideBase:
		ts = &q.baseThreads
	case vcs.SideHead:
		ts = &q.headThreads
	default:
		panic("unexpected vcs thread side")
	}

	// Search for a threadList having same start line.
	start, _ := c.Lines()
	list, _ := ts.Search(&threadsQuery{
		startLine: start,
	}).(*threadList)

	parent := c.Parent()
	switch {
	case list == nil && parent != nil:
		panic("no thread list found for child comment")

	case list == nil && parent == nil:
		// Append a new thread to a new threadList.
		v := new(threadList)
		v.push(vcs.Thread{c})
		*ts, _ = ts.Insert(v)

	case list != nil && parent == nil:
		// Append a new thread to an existing threadList.
		list.push(vcs.Thread{c})

	case list != nil && parent != nil:
		var found bool
		list.update(func(t vcs.Thread) vcs.Thread {
			for _, p := range t {
				if p.ID() == parent.ID() {
					found = true
					return append(t, c)
				}
			}
			return t
		})
		if !found {
			panic("no thread found for child comment")
		}
	}

	log.Printf("assigned comment id for comment %q: %x", c.ID(), q.commentID)
	q.commentIDs[c.ID()] = q.commentID
	q.commentID++
}

func (q *quick) Front() *editBuffer {
	return bufferFromElement(q.buffers.Front())
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
	for el := q.buffers.Front(); el != nil; el = el.Next() {
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

func (q *quick) printThread(w io.Writer, t vcs.Thread) {
	side := t.Side()
	if n, _ := t.Lines(); !q.headEdits[n] && !q.baseEdits[n] {
		side = vcs.SideBase
	}
	var prefixPad int
	switch side {
	case vcs.SideBase:
		prefixPad = 2
	case vcs.SideHead:
		prefixPad = 8
	default:
		panic("unexpected vcs thread side")
	}
	var (
		prefix = append(bytes.Repeat([]byte{' '}, prefixPad), "| "...)
		suffix = []byte(" |")
		titles = make([]string, len(t))
		bodies = make([]string, len(t))

		replacer = strings.NewReplacer(
			"\r\n", "\n",
			"\r", "\n",
		)

		maxLine int
	)
	for i, c := range t {
		body := strings.TrimSpace(c.Body())
		body = replacer.Replace(body)
		if n := ioutil.MaxLineRunesInString(color.FilterString(body)); n > maxLine {
			maxLine = n
		}
		bodies[i] = body + "\n"

		elapsed := timeutil.FormatSince(c.CreatedAt())
		if elapsed != "now" {
			elapsed += " ago"
		}
		title := fmt.Sprintf(
			"%s @%s, %s:\n",
			color.Sprintf(color.Grey, "[%s]", q.CommentID(c)),
			c.UserLogin(),
			elapsed,
		)
		if n := ioutil.MaxLineRunesInString(color.FilterString(title)); n > maxLine {
			maxLine = n
		}
		titles[i] = title
	}

	// 2 is for borders.
	// 2 is for padding to borders.
	padding := prefixPad + 2 + 2
	width := 80 - padding // Comment body width.
	if maxLine < width {
		width = maxLine
	}
	printSeparator := func(w io.Writer) {
		fmt.Fprintf(w,
			"%s+%s+\n",
			strings.Repeat(" ", prefixPad),
			strings.Repeat("-", width+padding-prefixPad-2),
		)
	}

	printSeparator(w)
	var buf bytes.Buffer
	for i := range t {
		if i > 0 {
			printSeparator(w)
		}
		var cw io.Writer
		cw = ioutil.NewLinePrefixWriter(&buf, prefix)
		cw = ioutil.NewLineSuffixWriter(cw, suffix)

		lw := ioutil.NewLineWrapWriter(cw, width)
		lw.SetPad(' ')
		lw.SetRuneCounter(func(p []byte) int {
			return utf8.RuneCount(color.Filter(p))
		})

		io.WriteString(lw, titles[i])
		io.WriteString(lw, bodies[i])
		lw.Flush()

		buf.WriteTo(w)
	}
	printSeparator(w)
}

func (q *quick) printBaseThreads(w io.Writer, line int) {
	ts, _ := q.baseThreads.Search(&threadsQuery{
		startLine: line,
	}).(*threadList)
	ts.forEach(func(t vcs.Thread) {
		q.printThread(w, t)
	})
}
func (q *quick) printHeadThreads(w io.Writer, line int) {
	ts, _ := q.headThreads.Search(&threadsQuery{
		startLine: line,
	}).(*threadList)
	ts.forEach(func(t vcs.Thread) {
		q.printThread(w, t)
	})
}

func (q *quick) printLine(w io.Writer, baseLine, headLine int, line []byte) {
	fmt.Fprintf(w,
		"  % 4d  % 4d %s\n",
		baseLine,
		headLine,
		bytes.TrimRight(line, "\n"),
	)
	q.printBaseThreads(w, baseLine)
	q.printHeadThreads(w, headLine)
}
func (q *quick) printHeadLine(w io.Writer, num int, line []byte) {
	color.Fprintf(w, color.Green,
		"+       % 4d %s\n",
		q.headLine,
		bytes.TrimRight(line, "\n"),
	)
	q.printHeadThreads(w, num)
}
func (q *quick) printBaseLine(w io.Writer, num int, line []byte) {
	color.Fprintf(w, color.Red,
		"- % 4d       %s\n",
		q.baseLine,
		bytes.TrimRight(line, "\n"),
	)
	q.printBaseThreads(w, num)
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

func (q *quick) CommentID(c vcs.Comment) string {
	id, ok := q.commentIDs[c.ID()]
	if !ok {
		panic("no comment id")
	}
	return strconv.FormatUint(uint64(id), 16)
}

func mixThreads(a, b []vcs.Thread) []vcs.Thread {
	ret := make([]vcs.Thread, 0, len(a)+len(b))
	var i, j int
	for i < len(a) && j < len(b) {
		at := a[i]
		bt := b[j]
		c := compareThreads(at, bt)
		switch {
		case c < 0:
			ret = append(ret, at)
			i++
		case c > 0:
			ret = append(ret, bt)
			j++
		default:
			i++
			j++
		}
	}
	ret = append(ret, a[i:]...)
	ret = append(ret, b[j:]...)
	return ret
}

func (q *quick) ThreadsBetween(baseStart, baseStop, headStart, headStop int) []vcs.Thread {
	return mixThreads(
		q.BaseThreadsBetween(baseStart, baseStop),
		q.HeadThreadsBetween(headStart, headStop),
	)
}

type compareFunc func(x avl.Item) int

func (f compareFunc) Compare(x avl.Item) int {
	return f(x)
}

func (q *quick) threadsBetween(tree avl.Tree, lo, hi int) (ret []vcs.Thread) {
	ts, _ := tree.Successor(compareFunc(func(x avl.Item) int {
		t := x.(*threadList)
		if t.startLine() >= lo {
			return -1
		}
		return +1
	})).(*threadList)
	for ts != nil && ts.startLine() < hi {
		// NOTE: ts are already sorted by date.
		ts.forEach(func(t vcs.Thread) {
			ret = append(ret, t)
		})
		ts, _ = tree.Successor(ts).(*threadList)
	}
	return ret
}

func (q *quick) BaseThreadsBetween(lo, hi int) (ret []vcs.Thread) {
	return q.threadsBetween(q.baseThreads, lo, hi)
}
func (q *quick) HeadThreadsBetween(lo, hi int) (ret []vcs.Thread) {
	return q.threadsBetween(q.headThreads, lo, hi)
}

func compareThreads(t0, t1 vcs.Thread) int {
	if c := compareInt(int(t0.Side()), int(t1.Side())); c != 0 {
		return c
	}
	start0, _ := t0.Lines()
	start1, _ := t1.Lines()
	if c := compareInt(start0, start1); c != 0 {
		return c
	}
	return 0
}

func compareThreadsByCreationTime(t0, t1 vcs.Thread) int {
	return compareTime(t0.CreatedAt(), t1.CreatedAt())
}

func (q *quick) ExpandBetween(w io.Writer, prev, next *editBuffer) (n int) {
	lo := prev.baseStop()
	hi := next.baseStart
	return q.expand(w, lo, hi, prev.headOffset)
}

func (q *quick) ExpandBefore(w io.Writer, e *editBuffer, lines int) (n int) {
	lo := e.baseStart - lines
	hi := e.baseStart
	if lo < 0 {
		lo = 0
	}
	var offset int
	if prev := q.Prev(e); prev != nil {
		offset = prev.headOffset
		if stop := prev.baseStop(); stop > lo {
			lo = stop
		}
	}
	return q.expand(w, lo, hi, offset)
}

func (q *quick) ExpandAfter(w io.Writer, e *editBuffer, lines int) (n int) {
	lo := e.baseStart + e.baseLines
	hi := lo + lines
	offset := e.headOffset
	if next := q.Next(e); next != nil {
		offset = next.headOffset
		if start := next.baseStart; start < hi {
			hi = start
		}
	}
	return q.expand(w, lo, hi, offset)
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
		q.printLine(w, lo, lo+headOffset, bts)
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
	q.printBaseLine(w, q.baseLine, line)
	q.baseEdits[q.baseLine] = true

	q.baseLine++
	q.baseLines++
	q.headOffset--
}

func (q *quick) advanceHead(n int) {
	q.headStart++
	q.headLine++
}

func (q *quick) insertLines(w io.Writer, cmd ed.Command) {
	var line []byte
	for text := cmd.Text; len(text) > 0; {
		line, text = split2(text, '\n')
		q.printHeadLine(w, q.headLine, line)
		q.headEdits[q.headLine] = true

		q.headLine++
		q.headLines++
		q.headOffset++
	}
}

func parseLineRange(s string) (side vcs.Side, lo, hi int, err error) {
	type parser func() (parser, error)
	var (
		sepParser  parser
		numParser  parser
		sideParser parser
	)
	var (
		num   int
		pos   int
		lines = [2]*int{&lo, &hi}
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

		// Propagate number to both lo and hi if we are on lo line now.
		for _, line := range lines[pos:] {
			*line = num
		}
		pos++
		if len(lines) == pos {
			return nil, nil
		}

		s = s[i:]
		return sepParser, nil
	}
	sideParser = func() (parser, error) {
		if len(s) == 0 {
			return nil, fmt.Errorf("malformed string")
		}
		switch s[0] {
		case '-':
			side = vcs.SideBase
		case '+':
			side = vcs.SideHead
		default:
			return nil, fmt.Errorf(
				"unexpected line type specifier: %q (+ or - are expected)",
				s[0],
			)
		}
		s = s[1:]
		return numParser, nil
	}
	for p := sideParser; p != nil; {
		p, err = p()
		if err != nil {
			return 0, 0, 0, err
		}
	}
	return side, lo, hi, nil
}

func split2(p []byte, c byte) (head, tail []byte) {
	i := bytes.IndexByte(p, c)
	if i == -1 {
		return p, nil
	}
	return p[:i], p[i+1:]
}

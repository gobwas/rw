package rw

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base32"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"golang.org/x/term"

	"github.com/gobwas/prompt"
	"github.com/gobwas/rw/color"
	"github.com/gobwas/rw/ed"
	rwioutil "github.com/gobwas/rw/ioutil"
	"github.com/gobwas/rw/vcs"
)

type Mode uint

const (
	ModeUnknown Mode = iota
	ModeQuick
	ModeDiff
	ModeCheckout
)

var (
	DefaultEditor     = "vimdiff"
	DefaultEditorArgs Args

	DefaultMode    = ModeQuick
	DefaultContext = 3

	TermInfo termInfo
)

func init() {
	for _, arg := range []string{
		"--clean",
		"{{ .Head }}",
		"{{ .Base }}",
	} {
		if err := DefaultEditorArgs.Set(arg); err != nil {
			panic(err)
		}
	}
	var err error
	TermInfo.Width, TermInfo.Height, err = term.GetSize(0)
	if err != nil {
		panic(err)
	}
}

type Args []string

func (as *Args) Set(s string) error {
	*as = append(*as, s)
	return nil
}

func (as *Args) String() string {
	var sb strings.Builder
	for _, s := range *as {
		sb.WriteByte(' ')
		sb.WriteString(s)
	}
	return sb.String()
}

type fileInfo struct {
	Name string
	Line int
}

type reviewInfo struct {
	HeadDir  string
	HeadFile fileInfo

	BaseDir  string
	BaseFile fileInfo

	PathSeparator string
}

func defaultTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"max": func(a, b int) int {
			if a > b {
				return a
			}
			return b
		},
		"min": func(a, b int) int {
			if a < b {
				return a
			}
			return b
		},
		"add": func(a, b int) int {
			return a + b
		},
		"percent": func(a, b int) int {
			return int(math.Round(float64(a) / float64(b) * 100))
		},
	}
}

type templateFuncer interface {
	TemplateFuncs() template.FuncMap
}

func compileArgs(args Args, data interface{}) (_ []string, err error) {
	ret := make([]string, 0, len(args))
	var sb strings.Builder
	for i, arg := range args {
		t := template.New("")
		t = t.Funcs(defaultTemplateFuncs())
		if f, ok := data.(templateFuncer); ok {
			t = t.Funcs(f.TemplateFuncs())
		}
		t, err = t.Parse(arg)
		if err != nil {
			return nil, err
		}
		if err := t.Execute(&sb, data); err != nil {
			return nil, err
		}
		log.Printf("compiled #%d arg: %#q", i, sb.String())
		if sb.Len() > 0 {
			ret = append(ret, sb.String())
		}
		sb.Reset()
	}
	return ret, nil
}

func (m *Mode) Set(s string) error {
	switch s {
	case "quick":
		*m = ModeQuick
	case "diff":
		*m = ModeDiff
	case "checkout":
		*m = ModeCheckout
	default:
		return fmt.Errorf("unknown review mode: %q", s)
	}
	return nil
}

func (m Mode) String() string {
	switch m {
	case ModeQuick:
		return "quick"
	case ModeDiff:
		return "diff"
	case ModeCheckout:
		return "checkout"
	default:
		return "<unknown>"
	}
}

type Review struct {
	Provider vcs.Provider
	Mode     Mode
	Preview  bool
	Comments bool

	ContextBefore int
	ContextAfter  int

	Editor     string
	EditorArgs Args

	Finder     string
	FinderArgs Args
}

type termInfo struct {
	Width  int
	Height int
}

type finderInfo struct {
	Prompt    string
	Multiple  bool
	NumOption int
	Term      termInfo
}

func (r *Review) selectFinder(ctx context.Context, message string, multiple bool, opts []prompt.Option) ([]int, error) {
	args, err := compileArgs(r.FinderArgs, finderInfo{
		Prompt:    message,
		Multiple:  multiple,
		NumOption: len(opts),
		Term:      TermInfo,
	})
	if err != nil {
		return nil, err
	}
	log.Printf("running finder: %s %s", r.Finder, args)
	return prompt.FinderSelect(ctx, opts, r.Finder, args...)
}

func (r *Review) selectSingle(ctx context.Context, message string, options []string) (int, error) {
	opts := prompt.SelectOptions(options...)
	var (
		xs  []int
		err error
	)
	if finder := r.Finder; finder != "" {
		xs, err = r.selectFinder(ctx, message, false, opts)
	} else {
		var x int
		x, err = prompt.SelectSingle(ctx, message, opts)
		xs = []int{x}
	}
	if err != nil {
		return -1, err
	}
	if len(xs) == 0 {
		return -1, fmt.Errorf("empty selection")
	}
	return xs[0], nil
}

func (r *Review) selectMultiple(ctx context.Context, message string, options []string) ([]int, error) {
	opts := prompt.SelectOptions(options...)
	var (
		xs  []int
		err error
	)
	if finder := r.Finder; finder != "" {
		xs, err = r.selectFinder(ctx, message, true, opts)
	} else {
		xs, err = prompt.SelectMultiple(ctx, message, opts)
	}
	if err != nil {
		return nil, err
	}
	return xs, nil
}

func (r *Review) contextBefore() int {
	if n := r.ContextBefore; n > 0 {
		return n
	}
	return DefaultContext
}
func (r *Review) contextAfter() int {
	if n := r.ContextAfter; n > 0 {
		return n
	}
	return DefaultContext
}

// root must exist.
func mkdirp(root string, dirpath string) error {
	for _, sub := range strings.Split(dirpath, "/") {
		root = path.Join(root, sub)
		info, err := os.Stat(root)
		if err == nil {
			if info.IsDir() {
				continue
			}
			return fmt.Errorf("file exists: %s", root)
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(root, 0744); err != nil {
			return err
		}
	}
	return nil
}

var enc = base32.HexEncoding.WithPadding(base32.NoPadding)

func hash(s string) string {
	sum := md5.Sum([]byte(s))
	return enc.EncodeToString(sum[:8])
}

func (r *Review) mode() Mode {
	if m := r.Mode; m != ModeUnknown {
		return m
	}
	return DefaultMode
}

func (r *Review) editorArgs() Args {
	if p := r.EditorArgs; p != nil {
		return p
	}
	return DefaultEditorArgs
}

func (r *Review) Start(ctx context.Context) error {
	review, err := r.selectReview(ctx)
	if err != nil {
		return err
	}
	switch r.mode() {
	case ModeQuick:
		return r.reviewQuick(ctx, review)
	case ModeDiff:
		return r.reviewDiff(ctx, review)
	case ModeCheckout:
		return r.reviewCheckout(ctx, review)
	default:
		return fmt.Errorf("unknown review mode")
	}
}

func (r *Review) reviewQuick(ctx context.Context, review vcs.Review) (err error) {
	changedFiles, err := review.ChangedFiles(ctx)
	if err != nil {
		return err
	}
	xs, err := r.selectMultiple(ctx, "Pick files to review:", changedFiles)
	if err != nil {
		return err
	}
	files := pick(changedFiles, xs...)

	tmp := temp{
		name: "rw",
	}
	for _, file := range files {
		baseSrc, err := review.BaseFile(ctx, file)
		if err != nil {
			return err
		}
		headSrc, err := review.HeadFile(ctx, file)
		if err != nil {
			return err
		}
		roBase, err := tmp.createFile(baseSrc, "base", file, 0444)
		if err != nil {
			return err
		}
		roHead, err := tmp.createFile(headSrc, "head", file, 0444)
		if err != nil {
			return err
		}
		var comments []vcs.Comment
		if r.Comments {
			comments, err = review.FileComments(ctx, file)
			if err != nil {
				return err
			}
		}
		var edits []ed.Command
		err = diff(ctx, roBase.Name(), roHead.Name(), func(cmd ed.Command) {
			edits = appendEdit(edits, cmd)
		})
		if err != nil {
			return err
		}

		q := newQuick(roBase, comments)
		q.Render(edits)

		for {
			color.Fprintf(os.Stdout, color.White, "index %s..%s\n",
				shortenRef(review.BaseName()),
				shortenRef(review.HeadName()),
			)
			color.Fprintf(os.Stdout, color.White, "--- %s\n",
				filepath.Join("a", file),
			)
			color.Fprintf(os.Stdout, color.White, "+++ %s\n",
				filepath.Join("b", file),
			)

			b := q.Front()
			for b != nil {
				var (
					lo = b
					hi = b
				)
				for prev, next := b, q.Next(b); next != nil && baseDistance(prev, next) <= r.contextAfter(); {
					hi = next
					prev = next
					next = q.Next(prev)
				}

			join:
				var (
					buffers     []io.WriterTo
					baseLines   int
					headLines   int
					staticLines int
				)
				for prev, curr := (*editBuffer)(nil), lo; curr != nil; prev, curr = curr, q.Next(curr) {
					baseLines += curr.baseLines
					headLines += curr.headLines

					if prev != nil {
						var buf bytes.Buffer
						staticLines += q.ExpandBetween(&buf, prev, curr)
						buffers = append(buffers, buffer(buf.Bytes()))
					}
					buffers = append(buffers, curr)

					if curr == hi {
						break
					}
				}
				var (
					before bytes.Buffer
					after  bytes.Buffer

					contextBefore = r.contextBefore()
					contextAfter  = r.contextAfter()
				)

			expand:
				var (
					beforeLines = q.ExpandBefore(&before, lo, contextBefore)
					afterLines  = q.ExpandAfter(&after, hi, contextAfter)
					expandLines = beforeLines + afterLines

					baseStart = lo.baseStart - beforeLines
					baseTotal = baseLines + staticLines + expandLines
					baseStop  = baseStart + baseTotal

					headStart = lo.headStart - beforeLines
					headTotal = headLines + staticLines + expandLines
					headStop  = headStart + headTotal
				)
				fmt.Fprintf(os.Stdout, "@@ -%d,%d +%d,%d:\n",
					baseStart, baseTotal,
					headStart, headTotal,
				)

				before.WriteTo(os.Stdout)
				for _, b := range buffers {
					b.WriteTo(os.Stdout)
				}
				after.WriteTo(os.Stdout)

				color.Println(color.Grey, strings.Repeat("~", 80))

			command:
				// TODO: move this to r.quiz()
				quiz := prompt.QuizOptions(
					"p", "Previous hunk",
					"n", "Next hunk",
					"q", "Quit immediately",
					"c", "Comment changes",
					"r", "Reply to a comment",
					"b", "Expand context before hunk",
					"a", "Expand context after hunk",
					"d", "Checkout a file and open a diff in an editor",
				)
				p := prompt.Quiz{
					Message: "What to do with this hunk",
					Options: quiz,
					//Prompt: prompt.Prompt{
					//	Output: writerFunc(func(p []byte) (int, error) {
					//		return color.Write(color.Blue, p)
					//	}),
					//},
				}
				i, err := p.Single(ctx)
				if err != nil {
					return err
				}
				switch quiz[i].Short {
				case "r":
					// TODO: move this to r.quiz()
					var quiz []prompt.Option
					for _, t := range q.ThreadsBetween(baseStart, baseStop, headStart, headStop) {
						for _, c := range t {
							quiz = append(quiz, prompt.Option{
								Short: q.CommentID(c),
								Data:  c,
							})
						}
					}
					s := prompt.Quiz{
						Message: "Reply to:",
						Options: quiz,
						//Prompt: prompt.Prompt{
						//	Output: writerFunc(func(p []byte) (int, error) {
						//		return color.Write(color.Blue, p)
						//	}),
						//},
					}
					i, err := s.Single(ctx)
					if err != nil {
						return err
					}
					body, err := prompt.ReadLine(ctx, "> ")
					if err != nil {
						return err
					}
					c := s.Options[i].Data.(vcs.Comment)
					rep, err := review.ReplyTo(ctx, c, body)
					if err != nil {
						return err
					}
					q.AppendComment(rep)

					for e := lo; e != q.Next(hi); e = q.Next(e) {
						q.RenderBuffer(e)
					}
					continue

				case "c":
				readRange:
					line, err := prompt.ReadLine(ctx, "Line(s) to comment: ")
					if err != nil {
						return err
					}
					side, lineLo, lineHi, err := parseLineRange(line)
					if err != nil {
						fmt.Printf("Bad input: %v\n", err)
						goto readRange
					}
					line, err = prompt.ReadLine(ctx, "> ")
					if err != nil {
						return err
					}
					c, err := review.Comment(ctx, file, side, lineLo, lineHi, line)
					if err != nil {
						return err
					}
					q.AppendComment(c)
					for e := lo; e != q.Next(hi); e = q.Next(e) {
						q.RenderBuffer(e)
					}
					continue

				case "b":
					if !q.HasLinesBefore(lo, beforeLines) {
						if p := q.Prev(lo); p != nil {
							lo = p
							goto join
						}
					} else {
						contextBefore += 5
					}
					goto expand

				case "a":
					if !q.HasLinesAfter(hi, afterLines) {
						if n := q.Next(hi); n != nil {
							hi = n
							goto join
						}
					} else {
						contextAfter += 5
					}
					goto expand

				case "p":
					prev := q.Prev(lo)
					if prev != nil {
						b = prev
						continue
					}
				case "n":
					next := q.Next(hi)
					if next != nil {
						b = next
						continue
					}

				case "d":
					// Checkout a head file to see a diff in an editor.
					f := checkoutFileLine(file, headStart)
					if err := r.checkout(ctx, review, f); err != nil {
						return err
					}
					goto command

				case "q":
					return nil
				}
				fmt.Printf("Reviewed all changes in %s.\n\n", color.Sprint(color.White, file))
				break
			}
			break
		}

	}

	return nil
}

func (r *Review) launchEditor(ctx context.Context, info reviewInfo) error {
	args, err := compileArgs(r.editorArgs(), info)
	if err != nil {
		return err
	}
	return launch(ctx, r.Editor, args...)
}

func applyEdit(comments []commentBlock, cmd ed.Command, apply func(ed.Command)) {
	log.Println("checking command", cmd.Start, cmd.End)
	log.Println(string(cmd.Text))

	i := sort.Search(len(comments), func(i int) bool {
		b := comments[i]
		// Find comment block that ends after of at the ed command start.
		//
		// +------------+        +------------+
		// | comment #1 |        | comment #2 |
		// +------------+        +------------+
		//                 +=====|~~~+        |
		//                 | comm|nd |        |
		//                 +=====|~~~+        |
		//                       |      +~~~~~|===+
		//                       |      | comm|nd |
		//                       |      +~~~~~|===+
		//                       |            +=========+
		//                       |            | command |
		//                       |            +=========+
		//                 +=====|~~~~~~~~~~~~|=============+
		//                 |     |      comman|             |
		//                 +=====|~~~~~~~~~~~~|=============+
		//
		// Any of these commands will be bound to the comment #2.
		// Parts that overlap with comment must be dropped.
		return b.line+b.size >= cmd.Start
	})
	if i == len(comments) {
		if i > 0 {
			var (
				bExtra = comments[i-1].extra
				bSize  = comments[i-1].size
			)
			cmd = shiftCommand(cmd, -bExtra-bSize)
		}
		apply(cmd)
		return
	}
	var (
		bExtra = comments[i].extra
		bSize  = comments[i].size
		bStart = comments[i].line
		bEnd   = bStart + bSize - 1
	)
	log.Printf(
		"found comment block: start=%d end=%d extra=%d",
		bSize, bEnd, bExtra,
	)
	if cmd.End < bStart {
		apply(shiftCommand(cmd, -bExtra))
		return
	}
	if cmd.Mode == ed.ModeAdd && cmd.Start == bEnd {
		// Add mode contains only address *after* which following text must
		// added. That is, that address may be the last line of the comment
		// block and its fine.
		apply(shiftCommand(cmd, -bExtra-bSize))
		return
	}
	if cmd.Start < bStart {
		c := cmd
		c.Text = rwioutil.TrimLinesRight(c.Text, 1+c.End-bStart)
		c.End = bStart - 1
		apply(shiftCommand(c, -bExtra))
	}
	if cmd.End > bEnd {
		c := cmd
		c.Text = rwioutil.TrimLinesLeft(c.Text, 1+bEnd-c.Start)
		c.Start = bEnd + 1
		apply(shiftCommand(c, -bExtra-bSize))
	}
}

func shiftCommand(cmd ed.Command, offset int) ed.Command {
	log.Println("shifting command", offset)
	cmd.Start += offset
	cmd.End += offset
	return cmd
}

type commentBlock struct {
	line  int
	size  int
	extra int
}

func annotate(src io.Reader, cs []vcs.Comment) (*os.File, []commentBlock, error) {
	temp, err := ioutil.TempFile("", "annotate")
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			temp.Close()
			os.Remove(temp.Name())
			return
		}
	}()
	ts := vcs.BuildThreads(cs)
	var (
		r = bufio.NewReader(src)
		w = bufio.NewWriter(temp)

		blocks    []commentBlock
		extraLine int
	)

	//w.WriteString("# vim:foldmethod=marker foldenable foldlevel=0\n")
	//blocks = append(blocks, commentBlock{
	//	line: 1,
	//	size: 1,
	//})
	//extraLine++

	linewrap := rwioutil.NewLineWrapWriter(nil, 80)

reading:
	for line, i := 1, 0; ; line++ {
		if i >= len(ts) {
			_, err := io.Copy(w, r)
			if err != nil {
				return nil, nil, err
			}
			break
		}
		for {
			line, isPrefix, err := r.ReadLine()
			if err == io.EOF {
				break reading
			}
			if err != nil {
				return nil, nil, err
			}
			// Not checking error here due that bufio.Writer provides sticky
			// error.
			w.Write(line)
			if !isPrefix {
				break
			}
		}

		// TODO: support \r\n here.
		w.WriteByte('\n')

		var (
			wrote bool
			block commentBlock
			sw    *statsWriter
		)
		for ; i < len(ts) && line == first(ts[i][0].Lines()); i++ {
			if !wrote {
				wrote = true
				block.line = extraLine + line + 1
				block.extra = extraLine
				sw = &statsWriter{
					w: w,
				}

				// TODO: use vim folding stuff here.
				//sw.WriteString("# review comments block {{{")
				sw.WriteString("/*")
				sw.WriteString(strings.Repeat("*", 78))
				sw.WriteString("\n")
			}
			for j, comment := range ts[i] {
				var dest io.Writer = sw
				if j > 0 {
					dest = &rwioutil.LinePrefixWriter{
						W:      dest,
						Prefix: bytes.Repeat([]byte{' '}, 4),
					}
				}
				// Use this counter to calculate number of bytes written in the
				// comment header.
				ssw := statsWriter{
					w: dest,
				}
				ssw.WriteString("\n")
				ssw.WriteString("@")
				ssw.WriteString(comment.UserLogin())
				ssw.WriteString(" at ")
				ssw.WriteString(comment.CreatedAt().Format(time.Stamp))
				if !comment.CreatedAt().Equal(comment.UpdatedAt()) {
					ssw.WriteString(" (updated at ")
					ssw.WriteString(comment.UpdatedAt().Format(time.Stamp))
					ssw.WriteString(")")
				}

				io.WriteString(dest, "\n#")
				io.WriteString(dest, comment.ID())
				io.WriteString(dest, ":\n")

				io.WriteString(dest, strings.Repeat("-", ssw.bytes))
				io.WriteString(dest, "\n")

				linewrap.Reset(dest)
				io.Copy(linewrap, strings.NewReader(comment.Body()))
				linewrap.Flush()

				sw.WriteString("\n")
			}
		}
		if wrote {
			sw.WriteString("\n")
			sw.WriteString(strings.Repeat("*", 78))
			sw.WriteString("*/\n")
			//sw.WriteString("# }}}\n")

			extraLine += sw.lines
			block.size = sw.lines
			blocks = append(blocks, block)
		}
	}
	if err := w.Flush(); err != nil {
		return nil, nil, err
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	return temp, blocks, nil
}

type statsWriter struct {
	w io.Writer

	lines int
	bytes int
}

func (w *statsWriter) Write(p []byte) (int, error) {
	w.lines += bytes.Count(p, []byte{'\n'})
	w.bytes += len(p)
	return w.w.Write(p)
}

func (w *statsWriter) WriteString(s string) (int, error) {
	w.lines += strings.Count(s, "\n")
	w.bytes += len(s)
	return io.WriteString(w.w, s)
}

func (r *Review) selectReview(ctx context.Context) (vcs.Review, error) {
	item, err := r.chooseReview(ctx)
	if err != nil {
		return nil, err
	}
	return r.Provider.Select(ctx, item)
}

func (r *Review) chooseReview(ctx context.Context) (vcs.ReviewItem, error) {
	var (
		items []vcs.ReviewItem
		opts  []string
	)
	err := r.Provider.List(ctx, func(item vcs.ReviewItem) {
		log.Printf("found review item: %s", item)
		items = append(items, item)
		opts = append(opts, item.String())
	})
	if err != nil {
		return nil, err
	}
	var i int
	switch len(opts) {
	case 0:
		return nil, fmt.Errorf("no review items")
	case 1:
		yes, err := prompt.Confirm(ctx, "Review `"+opts[0]+"`?")
		if err != nil {
			return nil, err
		}
		if !yes {
			return nil, io.EOF
		}
		i = 0

	default:
		i, err = r.selectSingle(ctx, "Choose a review:", opts)
		if err != nil {
			return nil, err
		}
	}

	return items[i], nil
}

func makeFile(name string, src io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

func sanitizePath(s string) string {
	return strings.Replace(s, string(os.PathSeparator), ".", -1)
}

func diff(ctx context.Context, prev, next string, fn func(ed.Command)) error {
	log.Println("executing", "diff", "--ed", "--text", prev, next)
	cmd := exec.CommandContext(ctx, "diff", "--ed", "--text", prev, next)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	err = ed.Diff(pipe, fn)

	e := cmd.Wait()
	if x, ok := e.(*exec.ExitError); ok && x.ProcessState.ExitCode() <= 1 {
		// Diff command returns 0 if no diff; 1 if there is a diff; >1 if is in
		// trouble.
		e = nil
	}
	if e != nil && err == nil {
		err = e
	}
	return err
}

func pick(opts []string, xs ...int) []string {
	ret := make([]string, len(xs))
	for i, x := range xs {
		ret[i] = opts[x]
	}
	return ret
}

func launch(ctx context.Context, name string, args ...string) error {
	log.Println("executing", name, argsString(args))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func argsString(args []string) string {
	var sb strings.Builder
	for i, arg := range args {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%#q", arg)
	}
	return sb.String()
}

func appendEditFunc(edits *[]ed.Command) func(ed.Command) {
	return func(cmd ed.Command) {
		*edits = appendEdit(*edits, cmd)
	}
}

func appendEdit(edits []ed.Command, cmd ed.Command) []ed.Command {
	log.Println("storing edit", cmd.Mode, cmd.Start, cmd.End)
	log.Println(string(cmd.Text))
	return append(edits, cmd)
}

func shortenRef(s string) string {
	if !isHash(s) || len(s) < 7 {
		return s
	}
	return s[:7]
}

func isHash(s string) bool {
	const toLower = 'a' - 'A'
	for i := 0; i < len(s); i++ {
		c := s[i] | toLower
		if 'a' <= c && c <= 'z' {
			continue
		}
		if '0' <= c && c <= '9' {
			continue
		}
		return false
	}
	return true
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

func first(a, b int) int {
	return a
}

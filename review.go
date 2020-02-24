package rw

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base32"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/gobwas/prompt"
	"github.com/gobwas/rw/ed"
	rwioutil "github.com/gobwas/rw/ioutil"
	"github.com/gobwas/rw/vcs"
)

type Mode uint

const (
	ModeUnknown Mode = iota
	ModeDiff
	ModeCheckout
)

var (
	DefaultCommand    = "vimdiff"
	DefaultParameters Parameters

	DefaultMode = ModeDiff
)

func init() {
	for _, arg := range []string{
		"--clean",
		"{{.Head}}",
		"{{.Base}}",
	} {
		if err := DefaultParameters.Set(arg); err != nil {
			panic(err)
		}
	}
}

type Parameters []*template.Template

func (ps *Parameters) Set(s string) error {
	t, err := template.New("").Parse(s)
	if err != nil {
		return err
	}
	*ps = append(*ps, t)
	return nil
}

func (ps *Parameters) String() string {
	var sb strings.Builder
	for _, p := range *ps {
		p.Execute(&sb, reviewInfo{
			Head: "<Head>",
			Base: "<Base>",
		})
		sb.WriteByte(' ')
	}
	return sb.String()
}

type reviewInfo struct {
	Head string
	Base string
}

func buildArgs(ps Parameters, r reviewInfo) ([]string, error) {
	ret := make([]string, len(ps))
	var sb strings.Builder
	for i, p := range ps {
		if err := p.Execute(&sb, r); err != nil {
			return nil, err
		}
		ret[i] = sb.String()
		sb.Reset()
	}

	return ret, nil
}

func (m *Mode) Set(s string) error {
	switch s {
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

	Command    string
	Parameters Parameters
}

func SetupReview(fs *flag.FlagSet) func() *Review {
	var r Review
	fs.Var(&r.Mode,
		"mode",
		"review mode",
	)
	fs.BoolVar(&r.Preview,
		"preview", false,
		"preview comments before send",
	)
	fs.BoolVar(&r.Comments,
		"comments", false,
		"annotate changed file with comments from vcs provider",
	)
	fs.StringVar(&r.Command,
		"command", DefaultCommand,
		"command to edit review",
	)
	fs.Var(&r.Parameters,
		"parameters",
		"parameters to be passed to the command; may support variables: Head, Base",
	)
	return func() *Review {
		if r.Parameters == nil {
			r.Parameters = DefaultParameters
		}
		if r.Mode == ModeUnknown {
			r.Mode = DefaultMode
		}
		return &r
	}
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

type temp struct {
	name string

	path string
	err  error
}

func (r *temp) init() {
	if r.err != nil {
		return
	}
	if r.path != "" {
		return
	}
	r.path, r.err = ioutil.TempDir("", r.name)
}

func (r *temp) file(src io.Reader, prefix, file string, mode os.FileMode) (filename string, err error) {
	r.init()
	if r.err != nil {
		return "", r.err
	}
	defer func() {
		log.Printf(
			"temp: created temp file: %s (%v err)",
			filename, err,
		)
	}()
	filename = path.Join(r.path, prefix, file)
	if mode&0222 == 0 {
		filename += ".ro"
	}
	r.err = mkdirp(r.path, strings.TrimPrefix(path.Dir(filename), r.path))
	if r.err != nil {
		return "", r.err
	}
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		r.err = err
		return "", err
	}
	defer f.Close()

	_, r.err = io.Copy(f, src)
	if r.err != nil {
		return "", r.err
	}

	return filename, nil
}

var enc = base32.HexEncoding.WithPadding(base32.NoPadding)

func hash(s string) string {
	sum := md5.Sum([]byte(s))
	return enc.EncodeToString(sum[:8])
}

func (r *Review) Start(ctx context.Context) error {
	review, err := r.selectReview(ctx)
	if err != nil {
		return err
	}
	switch r.Mode {
	case ModeDiff:
		return r.reviewDiff(ctx, review)
	case ModeCheckout:
		return r.reviewCheckout(ctx, review)
	default:
		return fmt.Errorf("unknown review mode")
	}
}

func (r *Review) reviewCheckout(ctx context.Context, review vcs.Review) (err error) {
	cleanup, err := review.Checkout(ctx)
	if err != nil {
		return err
	}
	defer func() {
		e := cleanup()
		if e != nil {
			log.Printf("warning: cleanup error: %v", err)
		}
		if err == nil {
			err = e
		}
	}()
	files, err := review.ChangedFiles(ctx)
	if err != nil {
		return err
	}

	xs, err := prompt.SelectMultiple(ctx, "Pick files to review:", files)
	if err != nil {
		return err
	}
	files = pick(files, xs...)

	tmp := temp{
		name: "rw",
	}

	// TODO: show side by side
	// TODO: annotate with comments

	var cmds []string
	for _, file := range files {
		base, err := review.BaseFile(ctx, file)
		if err != nil {
			return err
		}
		roBase, err := tmp.file(base, "base", file, 0444)
		if err != nil {
			return err
		}
		// Need to fixup deleted files here.
		if _, err := os.Stat(file); os.IsNotExist(err) {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			file = file + ".deleted"
			filename := path.Join(wd, file)
			if err := os.MkdirAll(path.Dir(filename), 0755); err != nil {
				return err
			}
			f, err := os.Create(filename)
			if err != nil {
				return err
			}
			f.Close()
			log.Printf("touched file %s", f.Name())
		}
		if r.Comments {
			head, err := review.HeadFile(ctx, file)
			if err != nil {
				return err
			}
			cs, err := review.FileComments(ctx, file)
			if err != nil {
				return err
			}
			f, _, err := annotate(head, cs)
			if err != nil {
				return err
			}
			if err := os.Rename(f.Name(), file); err != nil {
				return err
			}
			if os.Remove(f.Name()); err != nil {
				log.Printf("warning: remove temp file error: %v", err)
			}
		}
		cmds = append(cmds,
			"-c",
			fmt.Sprintf("tabnew %s", roBase),
			"-c",
			fmt.Sprintf("vert diffsplit %s", file),
		)
	}
	return launch(ctx, "vim", append([]string{
		"-c", "set background=dark",
		"-c", "let g:gitgutter_diff_base='" + review.BaseName() + "'",
		"-c", ":GitGutter",
	}, cmds...,
	)...)

	//return launch(ctx, "vim", append([]string{
	//	"-c", "set background=dark",
	//	"-c", "let g:gitgutter_diff_base='" + review.BaseName() + "'",
	//	"-c", ":GitGutter",
	//	"-c", "tabnew ",
	//	"-p"}, files...,
	//)...)
	//for _, file := range files {
	//	log.Println("LOCAL", file, "REMOTE", review.BaseFileName(file))
	//	err = r.launchEditor(ctx, reviewInfo{
	//		Head:  file,
	//		Base: review.BaseFileName(file),
	//	})
	//if err != nil {
	//	return err
	//}
	//}

	return nil
}

func (r *Review) reviewDiff(ctx context.Context, review vcs.Review) error {
	files, err := review.ChangedFiles(ctx)
	if err != nil {
		return err
	}

	xs, err := prompt.SelectMultiple(ctx, "Pick files to review:", files)
	if err != nil {
		return err
	}
	files = pick(files, xs...)

	tmp := temp{
		name: "rw",
	}
	for _, file := range files {
		baseSrc, err := review.BaseFile(ctx, file)
		if err != nil {
			return err
		}
		roBase, err := tmp.file(baseSrc, "base", file, 0444)
		if err != nil {
			return err
		}

		headSrc, err := review.HeadFile(ctx, file)
		if err != nil {
			return err
		}
		var (
			roHead string
			rwHead string

			comments []commentBlock
		)
		if r.Comments {
			cs, err := review.FileComments(ctx, file)
			if err != nil {
				return err
			}
			var head *os.File
			head, comments, err = annotate(headSrc, cs)
			if err != nil {
				return err
			}
			roHead, err = tmp.file(head, "head", file, 0444)
			if err != nil {
				return err
			}
			if _, err := head.Seek(0, io.SeekStart); err != nil {
				return err
			}
			rwHead, err = tmp.file(head, "head", file, 0644)
			if err != nil {
				return err
			}
		} else {
			bts, err := ioutil.ReadAll(headSrc)
			if err != nil {
				return err
			}
			roHead, err = tmp.file(bytes.NewReader(bts), "head", file, 0444)
			if err != nil {
				return err
			}
			rwHead, err = tmp.file(bytes.NewReader(bts), "head", file, 0644)
			if err != nil {
				return err
			}
		}

		//err = launch(ctx, "code", "--wait", "--diff", headFileEdit, baseFile)
		err = r.launchEditor(ctx, reviewInfo{
			Head: rwHead,
			Base: roBase,
		})
		if err != nil {
			return err
		}

		// NOTE: there is case when user adds two lines right before and right
		// after single comments block. In that case will be produced two edits
		// with same line range. For now its okay, but maybe it might be glued.
		var edits []ed.Command
		storeEdit := func(cmd ed.Command) {
			log.Println("storing edit", cmd.Start, cmd.End)
			log.Println(cmd.Text.String())
			var buf bytes.Buffer
			buf.ReadFrom(cmd.Text)
			cmd.Text = &buf
			edits = append(edits, cmd)
		}
		err = diff(ctx, roHead, rwHead, func(cmd ed.Command) {
			if comments != nil {
				applyEdit(comments, cmd, storeEdit)
			} else {
				storeEdit(cmd)
			}
		})
		if err != nil {
			return err
		}
		if r.Preview {
			// TODO If preview: create source file again from head; annotateIt with fake comments and show.
			// Fake comments might be created with provider.Preview(edit) -> vcs.Comment.
		}
		for _, cmd := range edits {
			log.Println("applying edit", cmd.Start, cmd.End)
			log.Println(cmd.Text.String())
			if err := review.Edit(ctx, file, cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Review) launchEditor(ctx context.Context, info reviewInfo) error {
	args, err := buildArgs(r.Parameters, info)
	if err != nil {
		return err
	}
	return launch(ctx, r.Command, args...)
}

func applyEdit(comments []commentBlock, cmd ed.Command, apply func(ed.Command)) {
	log.Println("checking command", cmd.Start, cmd.End)
	log.Println(cmd.Text.String())

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
	if cmd.Mode == ed.Add && cmd.Start == bEnd {
		// Add mode contains only address *after* which following text must
		// added. That is, that address may be the last line of the comment
		// block and its fine.
		apply(shiftCommand(cmd, -bExtra-bSize))
		return
	}
	if cmd.Start < bStart {
		c := cmd
		c.Text = lineSlice(c.Text, 0, 1+c.End-bStart)
		c.End = bStart - 1
		apply(shiftCommand(c, -bExtra))
	}
	if cmd.End > bEnd {
		c := cmd
		c.Text = lineSlice(c.Text, 1+bEnd-c.Start, 0)
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

func lineSlice(buf *bytes.Buffer, first, last int) *bytes.Buffer {
	log.Println("dropping command lines", first, last)
	lines := bytes.Split(buf.Bytes(), []byte{'\n'})
	ret := new(bytes.Buffer)
	for _, line := range lines[first : len(lines)-last-1] {
		ret.Write(line)
		ret.WriteByte('\n')
	}
	return ret
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
		for ; i < len(ts) && line == ts[i][0].Line(); i++ {
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
		i, err = prompt.SelectSingle(ctx, "Choose a review:", opts)
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
	log.Println("executing", name, args)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

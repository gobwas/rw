package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

var DefaultRepo Repository

func ShowRemote(ctx context.Context, remote string) (owner, repo string, err error) {
	return DefaultRepo.ShowRemote(ctx, remote)
}

func CurrentBranch(ctx context.Context) (string, error) {
	return DefaultRepo.CurrentBranch(ctx)
}

type Repository struct {
	Dir string
}

func Init(ctx context.Context, dir string) (*Repository, error) {
	r := new(Repository)
	r.Dir = dir
	err := r.Init(ctx)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Repository) Init(ctx context.Context) error {
	_, err := r.execute(ctx, "git", "init")
	return err
}

func (r *Repository) Clone(ctx context.Context, uri, upstream string) error {
	_, err := r.execute(ctx, "git", "clone", "--origin", upstream, uri, ".")
	return err
}

func (r *Repository) Fetch(ctx context.Context, remote string) error {
	_, err := r.execute(ctx, "git", "fetch", remote)
	return err
}

func (r *Repository) Pull(ctx context.Context) error {
	_, err := r.execute(ctx, "git", "pull")
	return err
}

func (r *Repository) Log(ctx context.Context, formats ...string) (lines [][]string, err error) {
	for _, f := range formats {
		if f == "%n" {
			return nil, fmt.Errorf("git: log: format %%n is non-supported")
		}
	}
	format := strings.Join(formats, "%n")
	out, err := r.execute(ctx, "git", "log", "--pretty="+format)
	if err != nil {
		return nil, err
	}
	for len(out) > 0 {
		line := make([]string, len(formats))
		for i := 0; i < len(formats); i++ {
			j := strings.IndexByte(out, '\n')
			if j == -1 {
				line[i] = out
				out = ""
				break
			}
			line[i], out = out[:j], out[j+1:]
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func (r *Repository) AddRemote(ctx context.Context, name, uri string) error {
	_, err := r.execute(ctx, "git", "remote", "add", name, uri)
	return err
}

func (r *Repository) ShowRemote(ctx context.Context, name string) (owner, repo string, err error) {
	s, err := r.execute(ctx, "git", "config", "--get",
		fmt.Sprintf("remote.%s.url", name),
	)
	if err != nil {
		return
	}
	u, err := parseGitURL(s)
	if err != nil {
		return
	}
	owner, repo = split2(u.Path, '/')
	repo = strings.TrimSuffix(repo, path.Ext(repo))
	return
}

func (r *Repository) EnsureRemote(ctx context.Context, name, uri string) error {
	act, _ := r.execute(ctx, "git", "config", "--get",
		fmt.Sprintf("remote.%s.url", name),
	)
	if act == "" {
		return r.AddRemote(ctx, name, uri)
	}
	if act == uri {
		return nil
	}
	return fmt.Errorf("remote %q is pointing to %q; not %q", name, act, uri)
}

func (r *Repository) DefaultBranch(ctx context.Context, remote string) (string, error) {
	str, err := r.execute(ctx, "git", "remote", "show", remote)
	if err != nil {
		return "", err
	}
	s := bufio.NewScanner(strings.NewReader(str))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		const prefix = "HEAD branch:"
		if ln := strings.TrimPrefix(line, prefix); ln != line {
			return strings.TrimSpace(ln), nil
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("git: malformed output")
}

func (r *Repository) ChangedFiles(ctx context.Context, base, head string) ([]string, error) {
	s, err := r.execute(ctx, "git", "diff", "--name-only", base+"..."+head)
	if err != nil {
		return nil, err
	}
	return strings.Split(s, "\n"), nil
}

func (r *Repository) ShowFile(ctx context.Context, branch, file string) (io.ReadCloser, error) {
	return r.stream(ctx, "git", "show", branch+":"+file)
}

func (r *Repository) CurrentBranch(ctx context.Context) (string, error) {
	s, err := r.execute(ctx, "git", "branch", "--no-color", "--show-current")
	return s, err
}

func (r *Repository) SwitchBranch(ctx context.Context, branch string) error {
	_, err := r.execute(ctx, "git", "checkout", branch)
	return err
}

func (r *Repository) TrackBranch(ctx context.Context, upstream, branch string) error {
	_, err := r.execute(ctx, "git", "branch",
		branch, path.Join(upstream, branch),
	)
	return err
}

func (r *Repository) EnsureTrackingBranch(ctx context.Context, upstream, branch string) error {
	_, err := r.execute(ctx, "git", "rev-parse", "--verify", branch)
	if err != nil {
		// We assume there is no such branch.
		return r.TrackBranch(ctx, upstream, branch)
	}
	// Otherwise check existing branch is points to a good upstream.
	act, err := r.execute(ctx, "git", "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	if err != nil {
		return err
	}
	exp := path.Join(upstream, branch)
	if act != exp {
		return fmt.Errorf(
			"git: tracking branch: branch %q tracks %q; not %q",
			branch, act, exp,
		)
	}
	return nil
}

func (r *Repository) Restore(ctx context.Context, files ...string) error {
	_, err := r.execute(ctx, "git", append([]string{"restore"}, files...)...)
	return err
}

func (r *Repository) CreateBranch(ctx context.Context, name string) error {
	_, err := r.execute(ctx, "git", "checkout", "-b", name)
	return err
}

func (r *Repository) DeleteBranch(ctx context.Context, name string) error {
	_, err := r.execute(ctx, "git", "branch", "-D", name)
	return err
}

func (r *Repository) Clean(ctx context.Context) error {
	_, err := r.execute(ctx, "git", "clean", "-f", "-d")
	return err
}

func (r *Repository) IsClean(ctx context.Context) (bool, error) {
	s, err := r.execute(ctx, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return s == "", nil
}

func (r *Repository) stream(ctx context.Context, name string, args ...string) (io.ReadCloser, error) {
	log.Println("streaming", name, args)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.Dir
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &commandPipe{
		cmd:  cmd,
		pipe: pipe,
	}, nil
}

func (r *Repository) execute(ctx context.Context, name string, args ...string) (output string, err error) {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = r.Dir
	defer func() {
		str := output
		if n := len(str); n > 128 {
			str = fmt.Sprintf("<too big output: %d bytes>", n)
		}
		log.Printf("exec done in %s: %s %s: %s %v", r.Dir, name, args, str, err)
	}()
	err = cmd.Run()
	if err != nil {
		var sb strings.Builder
		fmt.Fprintf(&sb, "exec `%s %s` error: %v", name, args, err)
		if stderr.Len() > 0 {
			fmt.Fprintf(&sb, ":\n%s", stderr.String())
		}
		return "", errors.New(sb.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

type commandPipe struct {
	cmd  *exec.Cmd
	pipe io.ReadCloser
}

func (c *commandPipe) Read(p []byte) (n int, err error) {
	defer func() {
		log.Println("read from pipe", n, err)
	}()
	return c.pipe.Read(p)
}

func (c *commandPipe) Close() error {
	if err := c.pipe.Close(); err != nil {
		return err
	}
	if err := c.cmd.Wait(); err != nil {
		return err
	}
	return nil
}

var scpSyntaxRe = regexp.MustCompile(`^([a-zA-Z0-9_]+)@([a-zA-Z0-9._-]+):(.*)$`)

func parseGitURL(s string) (u *url.URL, err error) {
	if m := scpSyntaxRe.FindStringSubmatch(s); m != nil {
		u = &url.URL{
			Scheme: "ssh",
			User:   url.User(m[1]),
			Host:   m[2],
			Path:   m[3],
		}
	} else {
		u, err = url.Parse(s)
	}
	return
}

func split2(s string, c byte) (string, string) {
	i := strings.IndexByte(s, c)
	if i == -1 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

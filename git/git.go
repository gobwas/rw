package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

func Remote(ctx context.Context, name string) (owner, repo string, err error) {
	s, err := execute(ctx, "git", "config", "--get",
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

func ChangedFiles(ctx context.Context, base, head string) ([]string, error) {
	s, err := execute(ctx, "git", "diff", "--name-only", base+"..."+head)
	if err != nil {
		return nil, err
	}
	return strings.Split(s, "\n"), nil
}

func ShowFile(ctx context.Context, branch, file string) (io.ReadCloser, error) {
	return stream(ctx, "git", "show", branch+":"+file)
}

func Branch(ctx context.Context) (string, error) {
	s, err := execute(ctx, "git", "branch", "--no-color", "--show-current")
	return s, err
}

func Checkout(ctx context.Context, branch string) error {
	_, err := execute(ctx, "git", "checkout", branch)
	return err
}

func Restore(ctx context.Context, files ...string) error {
	_, err := execute(ctx, "git", append([]string{"restore"}, files...)...)
	return err
}

func CheckoutBranch(ctx context.Context, name string) error {
	_, err := execute(ctx, "git", "checkout", "-b", name)
	return err
}

func DeleteBranch(ctx context.Context, name string) error {
	_, err := execute(ctx, "git", "branch", "-D", name)
	return err
}

func Clean(ctx context.Context) error {
	_, err := execute(ctx, "git", "clean", "-f", "-d")
	return err
}

func IsClean(ctx context.Context) (bool, error) {
	s, err := execute(ctx, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return s == "", nil
}

func stream(ctx context.Context, name string, args ...string) (io.ReadCloser, error) {
	log.Println("streaming", name, args)
	cmd := exec.CommandContext(ctx, name, args...)
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

func execute(ctx context.Context, name string, args ...string) (output string, err error) {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	defer func() {
		log.Println("executing", name, args, ":", output, err)
	}()
	err = cmd.Run()
	if err != nil {
		var sb strings.Builder
		fmt.Fprintf(&sb, "exec `%s %v` error: %v", name, args, err)
		if stderr.Len() > 0 {
			fmt.Fprintf(&sb, ":\n%s", stderr.String())
		}
		return "", fmt.Errorf(sb.String())
	}
	return strings.TrimSpace(stdout.String()), nil
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

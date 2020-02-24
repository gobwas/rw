package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path"
	"strconv"
	"strings"

	"github.com/gobwas/rw/ed"
	"github.com/gobwas/rw/vcs"
	"github.com/google/go-github/v39/github"
)

type pullRequest struct {
	c  *Client
	pr *github.PullRequest

	baseRemote string
	headRemote string

	comments comments
}

func (p *pullRequest) Close() error {
	return p.comments.Close()
}

func (p *pullRequest) String() string {
	return strings.Join([]string{
		p.c.owner,
		p.c.repo,
		p.base(),
		p.head(),
	}, "-")
}

func (p *pullRequest) ChangedFiles(ctx context.Context) ([]string, error) {
	return p.c.git.ChangedFiles(ctx, p.base(), p.head())
}

func (p *pullRequest) FileComments(ctx context.Context, file string) ([]vcs.Comment, error) {
	p.comments.Fetch(func(ctx context.Context) ([]*comment, error) {
		cs, _, err := p.c.client.PullRequests.ListComments(
			ctx, p.c.owner, p.c.repo, *p.pr.Number,
			&github.PullRequestListCommentsOptions{
				Sort:      "created",
				Direction: "asc",
			},
		)
		if err != nil {
			return nil, err
		}
		ret := make([]*comment, 0, len(cs))
		for _, c := range cs {
			if *c.CommitID != *p.pr.Head.SHA {
				// Outdated.
				continue
			}
			if c.Line == nil {
				// Comment not for the file.
				continue
			}
			ret = append(ret, prComment(c))
		}
		return ret, nil
	})
	return p.comments.File(ctx, file)
}

func (p *pullRequest) BaseName() string {
	return p.base()
}
func (p *pullRequest) HeadName() string {
	return p.head()
}

func (p *pullRequest) BaseFile(ctx context.Context, file string) (io.ReadCloser, error) {
	return p.c.git.ShowFile(ctx, p.base(), file)
}
func (p *pullRequest) HeadFile(ctx context.Context, file string) (io.ReadCloser, error) {
	return p.c.git.ShowFile(ctx, p.head(), file)
}

func (p *pullRequest) Checkout(ctx context.Context) (dir string, cleanup func() error, err error) {
	return checkout(ctx, p.c.git, *p.pr.Head.SHA)
}

func (p *pullRequest) Edit(ctx context.Context, file string, c ed.Command) (err error) {
	log.Printf("editing file %s", file)
	switch c.Mode {
	case ed.ModeDelete:
		return p.delete(ctx, file, c)
	case ed.ModeChange:
		return p.change(ctx, file, c)
	case ed.ModeAdd:
		return p.comment(ctx, file, c)
	}
	return nil
}

func (p *pullRequest) delete(ctx context.Context, file string, c ed.Command) error {
	for line := c.Start; line <= c.End; line++ {
		fmt.Println("SUGGEST DELETION AT", line)
		fmt.Println("----")
	}
	return nil
}
func (p *pullRequest) change(ctx context.Context, file string, c ed.Command) error {
	if c.Start != c.End {
		return fmt.Errorf("WARNING: multiline suggestions are not supported yet")
	}
	body := fmt.Sprintf(
		"```suggestion\n%s\n```",
		c.Text,
	)
	_, _, err := p.c.client.PullRequests.CreateComment(ctx, p.c.owner, p.c.repo, *p.pr.Number, &github.PullRequestComment{
		CommitID: p.pr.Head.SHA,
		Body:     &body,
		Path:     &file,
		Line:     &c.Start,
		Side:     &right,
	})
	return err
}

var (
	left  = "LEFT"
	right = "RIGHT"
)

func (p *pullRequest) ReplyTo(ctx context.Context, parent vcs.Comment, body string) (vcs.Comment, error) {
	c := parent.(*comment)
	r, _, err := p.c.client.PullRequests.CreateCommentInReplyTo(
		ctx, p.c.owner, p.c.repo, *p.pr.Number,
		body, c.id,
	)
	if err != nil {
		return nil, err
	}
	return prComment(r), nil
}

func sideOf(side vcs.Side) *string {
	s := &right
	if side == vcs.SideBase {
		s = &left
	}
	return s
}

func (p *pullRequest) Comment(
	ctx context.Context,
	file string,
	side vcs.Side,
	lo, hi int,
	body string,
) (c vcs.Comment, err error) {
	req := &github.PullRequestComment{
		CommitID: p.pr.Head.SHA,
		Body:     &body,
		Path:     &file,
		Side:     sideOf(side),
	}
	if lo < hi {
		req.StartLine = &lo
		req.Line = &hi
	} else {
		req.Line = &lo
	}
	defer func() {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("  ", "  ")
		enc.Encode(req)
		log.Printf(
			"create comment: %s:%d-%d (err %v)\n%s",
			file, lo, hi, err, buf.String(),
		)
	}()
	x, _, err := p.c.client.PullRequests.CreateComment(ctx, p.c.owner, p.c.repo, *p.pr.Number, req)
	if err != nil {
		return nil, err
	}
	return prComment(x), nil
}

func (p *pullRequest) comment(ctx context.Context, file string, c ed.Command) (err error) {
	defer func() {
		log.Printf(
			"sent comment: %s:%d (err %v)",
			file, c.Start, err,
		)
	}()
	body := string(bytes.TrimSpace(c.Text))
	if len(body) == 0 {
		log.Println("empty comment; skipping")
		return nil
	}

	// FIXME: move this logic outside of github concrete impl.
	switch body[0] {
	case '+':
		body = fmt.Sprintf(
			"```suggestion\n%s\n```",
			body[1:],
		)

	case '#':
		num, rest := split2(body[1:], ':')
		id, err := strconv.ParseInt(num, 10, 64)
		if err == nil {
			body = strings.TrimSpace(rest)
			_, _, err := p.c.client.PullRequests.CreateCommentInReplyTo(
				ctx, p.c.owner, p.c.repo, *p.pr.Number,
				body, id,
			)
			return err
		}
		log.Printf("warning: incorrect parent comment id: %q: %v", num, err)
	}

	start := c.Start

	_, _, err = p.c.client.PullRequests.CreateComment(ctx, p.c.owner, p.c.repo, *p.pr.Number, &github.PullRequestComment{
		CommitID: p.pr.Head.SHA,
		Body:     &body,
		Path:     &file,
		Line:     &start,
		Side:     &right,
	})
	return err
}

func (p *pullRequest) base() string {
	return path.Join(p.baseRemote, *p.pr.Base.Ref)
}

func (p *pullRequest) head() string {
	return path.Join(p.headRemote, *p.pr.Head.Ref)
}

func prComment(c *github.PullRequestComment) *comment {
	return &comment{
		id:        *c.ID,
		body:      *c.Body,
		startLine: parseInt(c.StartLine),
		line:      parseInt(c.Line),
		createdAt: *c.CreatedAt,
		updatedAt: *c.UpdatedAt,
		userLogin: *c.User.Login,
		side:      parseSide(c.Side),
		parentID:  parseInt64(c.InReplyTo),
		path:      *c.Path,
	}
}

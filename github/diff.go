package github

import (
	"context"
	"io"

	"github.com/gobwas/rw/ed"
	"github.com/gobwas/rw/vcs"
	"github.com/google/go-github/v39/github"
)

type diff struct {
	c      *Client
	commit *commit
	remote string

	comments comments
}

func (d *diff) String() string {
	return d.commit.String()
}

func (d *diff) Close() error {
	return d.comments.Close()
}

func (d *diff) Comment(ctx context.Context, file string, side vcs.Side, lo, hi int, body string) (vcs.Comment, error) {
	// TODO
	return nil, nil
}

// FIXME: remove it
func (d *diff) Edit(ctx context.Context, file string, cmd ed.Command) error {
	return nil
}

func (d *diff) ReplyTo(ctx context.Context, p vcs.Comment, body string) (vcs.Comment, error) {
	// TODO
	return nil, nil
}

func (d *diff) FileComments(ctx context.Context, file string) ([]vcs.Comment, error) {
	d.comments.Fetch(func(ctx context.Context) ([]*comment, error) {
		cs, _, err := d.c.client.Repositories.ListComments(
			ctx, d.c.owner, d.c.repo, nil,
		)
		if err != nil {
			return nil, err
		}
		ret := make([]*comment, 0, len(cs))
		for _, c := range cs {
			if *c.CommitID != d.commit.hash {
				continue
			}
			if c.Position == nil {
				// Comment not for the file.
				continue
			}
			ret = append(ret, repoComment(c))
		}
		return ret, nil
	})
	return d.comments.File(ctx, file)
}

func (d *diff) ChangedFiles(ctx context.Context) ([]string, error) {
	return d.c.git.ChangedFiles(ctx, d.BaseName(), d.HeadName())
}

func (d *diff) Checkout(ctx context.Context) (string, func() error, error) {
	return checkout(ctx, d.c.git, d.commit.hash)
}

func (d *diff) BaseFile(ctx context.Context, file string) (io.ReadCloser, error) {
	return d.c.git.ShowFile(ctx, d.BaseName(), file)
}
func (d *diff) HeadFile(ctx context.Context, file string) (io.ReadCloser, error) {
	return d.c.git.ShowFile(ctx, d.HeadName(), file)
}

func (d *diff) BaseName() string {
	return d.commit.parentHash
}
func (d *diff) HeadName() string {
	return d.commit.hash
}

func repoComment(c *github.RepositoryComment) *comment {
	return &comment{
		id:        *c.ID,
		body:      *c.Body,
		line:      *c.Position,
		createdAt: *c.CreatedAt,
		updatedAt: *c.UpdatedAt,
		userLogin: *c.User.Login,
		side:      vcs.SideBase,
		path:      *c.Path,
		//parentID:  parseInt64(c.InReplyTo),
	}
}

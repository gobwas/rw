package vcs

import (
	"context"
	"io"
	"time"

	"github.com/gobwas/rw/ed"
)

type Provider interface {
	List(context.Context, func(ReviewItem)) error
	Select(context.Context, ReviewItem) (Review, error)
}

type ReviewItem interface {
	String() string
}

type Review interface {
	ReviewItem

	ChangedFiles(context.Context) ([]string, error)
	FileComments(context.Context, string) ([]Comment, error)

	Checkout(context.Context) (cleanup func() error, err error)

	BaseFile(context.Context, string) (io.ReadCloser, error)
	HeadFile(context.Context, string) (io.ReadCloser, error)
	//BaseFileName(string) string
	//HeadFileName(string) string
	BaseName() string
	HeadName() string

	// TODO: differentiate base and head lines.
	Edit(ctx context.Context, file string, cmd ed.Command) error

	Close() error
}

type Comment interface {
	Line() int
	Body() string
	CreatedAt() time.Time
	UpdatedAt() time.Time
	UserLogin() string
	Compare(Comment) int
	Parent() Comment
	ID() string
}

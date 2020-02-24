package vcs

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gobwas/rw/ed"
)

type Provider interface {
	List(context.Context, func(ReviewItem)) error
	Select(context.Context, ReviewItem) (Review, error)
}

type ReviewItem interface {
	fmt.Stringer
}

type Review interface {
	ReviewItem

	ChangedFiles(context.Context) ([]string, error)
	FileComments(context.Context, string) ([]Comment, error)

	Checkout(context.Context) (workDir string, cleanup func() error, err error)

	BaseFile(context.Context, string) (io.ReadCloser, error)
	HeadFile(context.Context, string) (io.ReadCloser, error)
	BaseName() string
	HeadName() string

	Comment(ctx context.Context, file string, side Side, lo, hi int, body string) (Comment, error)
	ReplyTo(ctx context.Context, parent Comment, body string) (Comment, error)

	// TODO: differentiate base and head lines.
	// FIXME: this should be removed from review.
	Edit(ctx context.Context, file string, cmd ed.Command) error

	Close() error
}

type Side uint8

const (
	SideUnknown Side = iota
	SideBase
	SideHead
)

func (s Side) String() string {
	switch s {
	case SideBase:
		return "base"
	case SideHead:
		return "head"
	default:
		return "???"
	}
}

type Comment interface {
	Lines() (lo, hi int)
	Body() string
	Side() Side
	CreatedAt() time.Time
	UpdatedAt() time.Time
	UserLogin() string
	Parent() Comment
	ID() string
}

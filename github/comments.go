package github

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gobwas/rw/vcs"
)

type comments struct {
	once   sync.Once
	done   chan struct{}
	err    error
	m      map[string][]vcs.Comment
	ctx    context.Context
	cancel context.CancelFunc
}

func (c *comments) init() (first bool) {
	c.once.Do(func() {
		first = true
		c.done = make(chan struct{})
		c.m = make(map[string][]vcs.Comment)
		c.ctx, c.cancel = context.WithCancel(context.Background())
	})
	return
}

func (c *comments) File(ctx context.Context, file string) ([]vcs.Comment, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return c.m[file], nil
	}
}

func (c *comments) Close() error {
	var dummy bool
	c.once.Do(func() {
		dummy = true
	})
	if !dummy {
		c.cancel()
		<-c.done
	}
	return nil
}

func (c *comments) Fetch(fetch func(ctx context.Context) ([]*comment, error)) {
	if !c.init() {
		return
	}
	go func() {
		defer close(c.done)
		cs, err := fetch(c.ctx)
		if err != nil {
			c.err = err
			return
		}
		sort.Slice(cs, func(i, j int) bool {
			return cs[i].createdAt.Before(cs[j].createdAt)
		})
		index := make(map[int64]*comment)
		for _, x := range cs {
			//b := bytes.NewBuffer(nil)
			//e := json.NewEncoder(b)
			//e.SetIndent("", "  ")
			//e.Encode(c)
			//log.Printf("COMMENT FOR %s:\n%s\n\n", *c.Path, b.String())
			if x.parentID != 0 {
				// NOTE: we sorted comments sorted by creation date above.
				x.parent = index[x.parentID]
			}
			index[x.id] = x
			c.m[x.path] = append(c.m[x.path], x)
		}
	}()
}

type comment struct {
	parent *comment

	id        int64
	body      string
	startLine int
	line      int
	createdAt time.Time
	updatedAt time.Time
	userLogin string
	side      vcs.Side
	parentID  int64
	path      string
}

func (c *comment) Lines() (lo, hi int) {
	var (
		start = c.startLine
		line  = c.line
	)
	if start != 0 {
		lo = start
	} else {
		lo = line
	}
	hi = line
	return lo, hi
}
func (c *comment) Body() string {
	return c.body
}
func (c *comment) CreatedAt() time.Time {
	return c.createdAt
}
func (c *comment) UpdatedAt() time.Time {
	return c.updatedAt
}
func (c *comment) UserLogin() string {
	return c.userLogin
}
func (c *comment) Side() vcs.Side {
	return c.side
}
func (c *comment) Parent() vcs.Comment {
	if c.parent != nil {
		return c.parent
	}
	return nil
}
func (c *comment) ID() string {
	return strconv.FormatInt(c.id, 10)
}

func parseSide(s *string) vcs.Side {
	if s != nil && *s == "LEFT" {
		return vcs.SideBase
	}
	return vcs.SideHead
}

func parseInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func parseInt64(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

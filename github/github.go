package github

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/rw/ed"
	"github.com/gobwas/rw/git"
	"github.com/gobwas/rw/vcs"
	"github.com/google/go-github/v29/github"
	"golang.org/x/oauth2"
)

type Client struct {
	Token      string
	Remote     string
	User       string
	PRID       int
	PRTemplate string

	OnOctocat func(string)

	qualifiers qualifiers

	once       sync.Once
	client     *github.Client
	owner      string
	repo       string
	err        error
	prTemplate *template.Template
}

func (c *Client) Init(ctx context.Context) error {
	c.once.Do(func() {
		c.prTemplate, c.err = template.New("pr").Parse(c.PRTemplate)
		if c.err != nil {
			return
		}
		c.owner, c.repo, c.err = git.Remote(ctx, c.Remote)
		if c.err != nil {
			return
		}
		c.qualifiers.Set("is:open")
		c.qualifiers.Set(fmt.Sprintf(
			"repo:%s/%s", c.owner, c.repo,
		))
		if user := c.User; user != "" {
			c.qualifiers.Set(fmt.Sprintf(
				"review-requested:%s", user,
			))
		}

		ts := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: c.Token,
			},
		)
		c.client = github.NewClient(oauth2.NewClient(ctx, ts))
		if c.err = c.ping(ctx); c.err != nil {
			return
		}
	})
	return c.err
}

func (c *Client) ping(ctx context.Context) error {
	octocat, _, err := c.client.Octocat(ctx, "")
	if err != nil {
		return err
	}
	if fn := c.OnOctocat; fn != nil {
		fn(octocat)
	}
	return nil
}

func (c *Client) Select(ctx context.Context, item vcs.ReviewItem) (vcs.Review, error) {
	var p *PullRequest
	switch v := item.(type) {
	case *issue:
		id, err := prIDFromURL(*v.issue.PullRequestLinks.URL)
		if err != nil {
			return nil, err
		}
		p, err = c.pullRequest(ctx, id)
		if err != nil {
			return nil, err
		}

	case *PullRequest:
		p = v
	}
	if *p.pr.Head.Repo.ID != *p.pr.Base.Repo.ID {
		return nil, fmt.Errorf("github: can't compare between different remotes yet")
	}
	return p, nil
}

func (c *Client) List(ctx context.Context, fn func(vcs.ReviewItem)) error {
	if id := c.PRID; id != 0 {
		pr, err := c.pullRequest(ctx, id)
		if err != nil {
			return err
		}
		fn(pr)
		return nil
	}
	query := c.qualifiers.String()
	r, _, err := c.client.Search.Issues(ctx, query, &github.SearchOptions{
		Sort:  "created",
		Order: "asc",
	})
	if err != nil {
		return err
	}
	for _, iss := range r.Issues {
		if !iss.IsPullRequest() {
			continue
		}
		fn(&issue{
			issue:    iss,
			template: c.prTemplate,
		})
	}
	return nil
}

func (c *Client) AddQualifier(s string) error {
	return c.qualifiers.Set(s)
}

func (c *Client) pullRequest(ctx context.Context, id int) (*PullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, id)
	if err != nil {
		return nil, err
	}
	return &PullRequest{
		c:  c,
		pr: pr,
	}, nil
}

type PullRequest struct {
	c  *Client
	pr *github.PullRequest

	once     sync.Once
	ctx      context.Context
	cancel   context.CancelFunc
	comments *comments
}

func (p *PullRequest) Close() error {
	var dummy bool
	p.once.Do(func() {
		dummy = true
	})
	if !dummy {
		p.cancel()
		<-p.comments.done
	}
	return nil
}

func (p *PullRequest) String() string {
	return strings.Join([]string{
		p.c.owner,
		p.c.repo,
		p.base(),
		p.head(),
	}, "-")
}

func (p *PullRequest) ChangedFiles(ctx context.Context) ([]string, error) {
	files, err := git.ChangedFiles(ctx, p.base(), p.head())
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (p *PullRequest) FileComments(ctx context.Context, file string) ([]vcs.Comment, error) {
	p.fetchComments()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.comments.done:
	}
	return p.comments.m[file], nil
}

type comments struct {
	done chan struct{}
	err  error
	m    map[string][]vcs.Comment
}

func (p *PullRequest) fetchComments() {
	p.once.Do(func() {
		p.ctx, p.cancel = context.WithCancel(context.Background())
		p.comments = &comments{
			done: make(chan struct{}),
			m:    make(map[string][]vcs.Comment),
		}
		go func() {
			defer close(p.comments.done)

			index := make(map[int64]*comment)

			cs, _, err := p.c.client.PullRequests.ListComments(
				p.ctx, p.c.owner, p.c.repo, *p.pr.Number,
				&github.PullRequestListCommentsOptions{
					Sort:      "created",
					Direction: "asc",
				},
			)
			if err != nil {
				p.comments.err = err
				return
			}

			for _, c := range cs {
				if *c.CommitID != *p.pr.Head.SHA {
					// Outdated.
					continue
				}
				if c.Line == nil {
					// Comment not for the file.
					continue
				}

				v := &comment{
					comment: c,
				}
				if c.InReplyTo != nil {
					v.parent = index[*c.InReplyTo]
				}
				index[*c.ID] = v
				p.comments.m[*c.Path] = append(p.comments.m[*c.Path], v)
			}
		}()
	})
}

type comment struct {
	comment *github.PullRequestComment
	parent  *comment
}

func (c *comment) Line() int {
	return *c.comment.Line
}
func (c *comment) Body() string {
	return *c.comment.Body
}
func (c *comment) CreatedAt() time.Time {
	return *c.comment.CreatedAt
}
func (c *comment) UpdatedAt() time.Time {
	return *c.comment.UpdatedAt
}
func (c *comment) UserLogin() string {
	return *c.comment.User.Login
}
func (c *comment) Parent() vcs.Comment {
	if c.parent != nil {
		return c.parent
	}
	return nil
}
func (c *comment) Compare(b vcs.Comment) int {
	return int(*c.comment.ID - *b.(*comment).comment.ID)
}
func (c *comment) ID() string {
	return strconv.FormatInt(*c.comment.ID, 10)
}

func (p *PullRequest) BaseFileName(file string) string {
	return p.base() + "/" + file
}
func (p *PullRequest) HeadFileName(file string) string {
	return p.head() + "/" + file
}
func (p *PullRequest) BaseName() string {
	return p.base()
}
func (p *PullRequest) HeadName() string {
	return p.head()
}

func (p *PullRequest) BaseFile(ctx context.Context, file string) (io.ReadCloser, error) {
	return git.ShowFile(ctx, p.base(), file)
}
func (p *PullRequest) HeadFile(ctx context.Context, file string) (io.ReadCloser, error) {
	return git.ShowFile(ctx, p.head(), file)
}

func (p *PullRequest) Checkout(ctx context.Context) (_ func() error, err error) {
	master, err := git.Branch(ctx)
	if err != nil {
		return nil, err
	}
	cleanup := func() error {
		return git.Checkout(ctx, master)
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	clean, err := git.IsClean(ctx)
	if err != nil {
		return nil, err
	}
	if !clean {
		return nil, fmt.Errorf(
			"github: can't git checkout: working tree is not clean",
		)
	}
	if err := git.Checkout(ctx, *p.pr.Head.SHA); err != nil {
		return nil, err
	}
	// TODO
	branch := "random-branch-name-for-review"
	if err := git.CheckoutBranch(ctx, branch); err != nil {
		return nil, err
	}
	return func() error {
		e0 := git.Restore(context.TODO(), ".")
		e1 := git.Clean(context.TODO())
		e2 := git.Checkout(ctx, master)
		e3 := git.DeleteBranch(context.TODO(), branch)
		if e0 != nil {
			return e0
		}
		if e1 != nil {
			return e1
		}
		if e2 != nil {
			return e2
		}
		return e3
	}, nil
}

func (p *PullRequest) Edit(ctx context.Context, file string, c ed.Command) (err error) {
	log.Printf("editing file %s", file)
	switch c.Mode {
	case ed.Delete:
		return p.delete(ctx, file, c)
	case ed.Change:
		return p.change(ctx, file, c)
	case ed.Add:
		return p.comment(ctx, file, c)
	}
	return nil
}

func (p *PullRequest) delete(ctx context.Context, file string, c ed.Command) error {
	for line := c.Start; line <= c.End; line++ {
		fmt.Println("SUGGEST DELETION AT", line)
		fmt.Println("----")
	}
	return nil
}
func (p *PullRequest) change(ctx context.Context, file string, c ed.Command) error {
	if c.Start != c.End {
		return fmt.Errorf("WARNING: multiline suggestions are not supported yet")
	}
	body := fmt.Sprintf(
		"```suggestion\n%s\n```",
		c.Text.String(),
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

func (p *PullRequest) comment(ctx context.Context, file string, c ed.Command) (err error) {
	defer func() {
		log.Printf(
			"sent comment: %s:%d (err %v)",
			file, c.Start, err,
		)
	}()
	body := strings.TrimSpace(c.Text.String())
	if len(body) == 0 {
		log.Println("empty comment; skipping")
		return nil
	}

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

	_, _, err = p.c.client.PullRequests.CreateComment(ctx, p.c.owner, p.c.repo, *p.pr.Number, &github.PullRequestComment{
		CommitID: p.pr.Head.SHA,
		Body:     &body,
		Path:     &file,
		Line:     &c.Start,
		Side:     &right,
	})
	return err
}

func (p *PullRequest) base() string {
	return *p.pr.Base.SHA
	//return p.c.Remote + "/" + *p.pr.Base.Ref
}

func (p *PullRequest) head() string {
	return *p.pr.Head.SHA
	//return p.c.Remote + "/" + *p.pr.Head.Ref
}

type qualifiers struct {
	sb strings.Builder
}

func (qs *qualifiers) Set(s string) error {
	if qs.sb.Len() > 0 {
		qs.sb.WriteByte(' ')
	}
	qs.sb.WriteString(s)
	return nil
}

func (qs *qualifiers) String() string {
	return qs.sb.String()
}

type issue struct {
	issue    github.Issue
	template *template.Template
}

func (s *issue) String() string {
	var sb strings.Builder
	err := s.template.Execute(&sb, s)
	if err != nil {
		panic(err)
	}
	return sb.String()
}

func (s *issue) UserLogin() string {
	return *s.issue.User.Login
}

func (s *issue) Title() string {
	return *s.issue.Title
}

func prIDFromURL(s string) (int, error) {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return 0, err
	}
	p := strings.Split(u.Path, "/")
	s = p[len(p)-1]

	id, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

func split2(s string, c byte) (string, string) {
	i := strings.IndexByte(s, c)
	if i == -1 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

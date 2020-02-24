package github

import (
	"context"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gobwas/rw/git"
	"github.com/gobwas/rw/vcs"
	"github.com/google/go-github/v39/github"
	"golang.org/x/oauth2"
)

type Client struct {
	Project    string
	Commits    bool
	CacheDir   string
	Token      string
	Origin     string
	Branch     string
	User       string
	PRID       int
	PRTemplate string

	OnOctocat func(string)

	qualifiers qualifiers

	once       sync.Once
	git        *git.Repository
	client     *github.Client
	owner      string
	repo       string
	err        error
	prTemplate *template.Template
}

const cacheOrigin = "rw-origin"

func (c *Client) Init(ctx context.Context) error {
	c.once.Do(func() {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: c.Token,
			},
		)
		c.client = github.NewClient(oauth2.NewClient(ctx, ts))
		if c.err = c.ping(ctx); c.err != nil {
			return
		}

		c.prTemplate, c.err = template.New("pr").Parse(c.PRTemplate)
		if c.err != nil {
			return
		}

		branch := c.Branch
		if p := c.Project; p != "" {
			c.owner, c.repo = split2(p, '/')
			if c.repo == "" {
				c.err = fmt.Errorf("malformed project name: %q", p)
				return
			}
		} else {
			// Try to work with repo in process's current directory.
			c.owner, c.repo, c.err = git.ShowRemote(ctx, c.Origin)
			if c.err == nil {
				branch, c.err = git.CurrentBranch(ctx)
			}
			if c.err != nil {
				return
			}
		}

		// Unconditionally create temp dir with repo.
		// We don't want to mutate remotes for existing repo.
		var dir string
		if cache := c.CacheDir; cache == "" {
			dir, c.err = ioutil.TempDir("", "rw*")
		} else {
			dir = filepath.Join(cache, c.owner, c.repo)
			c.err = os.MkdirAll(dir, 0755)
		}
		if c.err != nil {
			return
		}

		c.git = &git.Repository{
			Dir: dir,
		}
		// NOTE: Run `git init` twice is safe here.
		if c.err = c.git.Init(ctx); c.err != nil {
			return
		}
		origin := fmt.Sprintf("git@github.com:%s/%s.git", c.owner, c.repo)
		if c.err = c.git.EnsureRemote(ctx, cacheOrigin, origin); c.err != nil {
			return
		}
		if c.err = c.git.Fetch(ctx, cacheOrigin); c.err != nil {
			return
		}

		if branch == "" {
			branch, c.err = c.git.DefaultBranch(ctx, cacheOrigin)
			if c.err != nil {
				return
			}
		}
		c.err = c.git.EnsureTrackingBranch(ctx, cacheOrigin, branch)
		if c.err != nil {
			return
		}
		c.err = c.git.SwitchBranch(ctx, branch)
		if c.err != nil {
			return
		}
		c.err = c.git.Pull(ctx)
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

func splitURL(s string) (owner, repo string, err error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", "", err
	}
	owner, repo = split2(u.Path, '/')
	return owner, repo, nil
}

func remoteName(cloneURL string) (string, error) {
	owner, repo, err := splitURL(cloneURL)
	if err != nil {
		return "", err
	}
	repo = strings.TrimSuffix(repo, path.Ext(repo))
	return path.Join(strings.ToLower(owner), strings.ToLower(repo)), nil
}

func (c *Client) Select(ctx context.Context, item vcs.ReviewItem) (vcs.Review, error) {
	switch v := item.(type) {
	case *issue:
		id, err := prIDFromURL(*v.issue.PullRequestLinks.URL)
		if err != nil {
			return nil, err
		}
		p, err := c.pullRequest(ctx, id)
		if err != nil {
			return nil, err
		}

		p.baseRemote = cacheOrigin
		if *p.pr.Base.Repo.ID == *p.pr.Head.Repo.ID {
			p.headRemote = cacheOrigin
			return p, nil
		}

		remote, err := remoteName(*p.pr.Head.Repo.CloneURL)
		if err != nil {
			return nil, err
		}
		err = c.git.EnsureRemote(ctx, remote, *p.pr.Head.Repo.CloneURL)
		if err == nil {
			err = c.git.Fetch(ctx, remote)
		}
		if err != nil {
			return nil, err
		}
		p.headRemote = remote

		return p, nil

		//case *PullRequest:
		//	p = v

	case *commit:
		return &diff{
			c:      c,
			commit: v,
			remote: cacheOrigin,
		}, nil

	default:
		return nil, fmt.Errorf("github: select: unsupported type: %T", v)
	}
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
	if c.Commits {
		lines, err := c.git.Log(ctx, "%P", "%H", "%h", "%at", "%aL", "%s")
		if err != nil {
			return err
		}
		for _, line := range lines {
			fn(&commit{
				parentHash: line[0],
				hash:       line[1],
				shortHash:  line[2],
				email:      line[4],
				title:      line[5],
			})
		}
		return nil
	}
	query := c.qualifiers.String()
	options := &github.SearchOptions{
		Sort:  "created",
		Order: "asc",
		ListOptions: github.ListOptions{
			Page:    0,
			PerPage: 100,
		},
	}
	for {
		result, resp, err := c.client.Search.Issues(ctx, query, options)
		if err != nil {
			return err
		}
		for _, iss := range result.Issues {
			if !iss.IsPullRequest() {
				continue
			}
			fn(&issue{
				issue:    iss,
				template: c.prTemplate,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		options.ListOptions.Page = resp.NextPage
	}
	return nil
}

func (c *Client) AddQualifier(s string) error {
	return c.qualifiers.Set(s)
}

func (c *Client) pullRequest(ctx context.Context, id int) (*pullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, id)
	if err != nil {
		return nil, err
	}
	return &pullRequest{
		c:  c,
		pr: pr,
	}, nil
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
	issue    *github.Issue
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

type commit struct {
	parentHash string
	hash       string
	shortHash  string
	email      string
	title      string
}

func (c *commit) String() string {
	return fmt.Sprintf("%s: %s: %s", c.shortHash, c.email, c.title)
}

func (c *commit) UserLogin() string {
	return c.email
}

func (c *commit) Title() string {
	return c.title
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

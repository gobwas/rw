package github

import (
	"flag"
	"fmt"
)

func SetupClient(fs *flag.FlagSet) func() *Client {
	var (
		c       Client
		octocat bool
	)
	fs.StringVar(&c.Token,
		"token", "",
		"personal api token",
	)
	fs.Var(&c.qualifiers,
		"q",
		"issue search qualifier",
	)
	fs.StringVar(&c.User,
		"user", "",
		"user to whom review is requested",
	)
	fs.IntVar(&c.PRID,
		"pr", 0,
		"pull request id",
	)
	fs.StringVar(&c.PRTemplate,
		"pr-template", `@{{ .UserLogin }}: {{ .Title }}`,
		"pull request template",
	)
	fs.StringVar(&c.Remote,
		"remote", "origin",
		"name of the git remote",
	)
	fs.BoolVar(&octocat,
		"octocat", false,
		"draw octocat on initialization",
	)
	return func() *Client {
		if octocat {
			c.OnOctocat = func(s string) {
				fmt.Println(s)
			}
		}
		return &c
	}
}

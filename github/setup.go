package github

import (
	"flag"
)

func DefineFlags(c *Client, fs *flag.FlagSet) {
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
	fs.StringVar(&c.Origin,
		"origin", "origin",
		"name of the git remote upstream to use",
	)
}

package main

import (
	"context"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/gobwas/cli"
	"github.com/gobwas/flagutil"
	"github.com/gobwas/flagutil/parse/env"
	"github.com/gobwas/flagutil/parse/file"
	"github.com/gobwas/flagutil/parse/file/yaml"
	"github.com/gobwas/flagutil/parse/pargs"
	"github.com/gobwas/rw"
	"github.com/gobwas/rw/github"
)

var _ interface {
	cli.Command
	cli.FlagDefiner
} = (*command)(nil)

type command struct {
	cacheDir string
	project  string
	branch   string
	commits  bool
	debug    bool

	github github.Client
	review rw.Review
}

func (c *command) DefineFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.debug,
		"debug", false,
		"print debug logs",
	)
	fs.StringVar(&c.project,
		"project", "",
		"project to lookup review in",
	)
	fs.BoolVar(&c.commits,
		"commits", false,
		"review commits instead of pull-requests",
	)
	fs.StringVar(&c.branch,
		"branch", "",
		"branch to use for --commits flag",
	)
	fs.StringVar(&c.cacheDir,
		"cache", cacheDir(),
		"where to store cached repos",
	)

	// Set the default config flag value.
	_ = fs.String("config",
		filepath.Join(configDir(), "config.yaml"),
		"path to a configuration file",
	)

	rw.DefineFlags(&c.review, fs)

	flagutil.Subset(fs, "github", func(fs *flag.FlagSet) {
		github.DefineFlags(&c.github, fs)
	})
}

func (c *command) Run(ctx context.Context, args []string) error {
	log.SetFlags(log.Lshortfile)
	if !c.debug {
		log.SetOutput(ioutil.Discard)
	}

	c.github.Project = c.project
	c.github.Commits = c.commits
	c.github.Branch = c.branch
	c.github.CacheDir = c.cacheDir
	if err := c.github.Init(ctx); err != nil {
		return err
	}

	c.review.Provider = &c.github
	if err := c.review.Start(ctx); err != nil {
		return err
	}

	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())

	r := cli.Runner{
		// Override flags parsing to use flagutil package. It allows us to have
		// fancy things like flag shortucts and posix-compatible flags syntax.
		DoParseFlags: func(ctx context.Context, fs *flag.FlagSet, args []string) ([]string, error) {
			opts, rest := flagParseOptions(fs, args)
			err := flagutil.Parse(ctx, fs, opts...)
			if err != nil {
				return nil, err
			}
			return rest(), err
		},
		// Override help message printing. It will print pretty help message
		// which is aware of flag shortcuts used by flagutil package.
		DoPrintFlags: func(ctx context.Context, w io.Writer, fs *flag.FlagSet) error {
			// Be kind and restore original output writer.
			orig := fs.Output()
			fs.SetOutput(w)
			defer fs.SetOutput(orig)

			// Note that to print right help message we have to use same parse
			// options we used in DoParseFlags() above. That's why here is
			// parseOptions() helper func.
			opts, _ := flagParseOptions(fs, nil)

			return flagutil.PrintDefaults(ctx, fs, opts...)
		},
	}
	r.Main(new(command))
}

func mustDir(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

func homeDir() (string, error) {
	u, err := user.Current()
	if err == nil {
		return u.HomeDir, nil
	}
	h, has := os.LookupEnv("HOME")
	if !has {
		return "", err
	}
	return h, nil
}

func cacheDir() string {
	return filepath.Join(mustDir(homeDir()), ".cache", "rw")
}

func configDir() string {
	return filepath.Join(mustDir(homeDir()), ".config", "rw")
}

func flagParseOptions(fs *flag.FlagSet, args []string) (
	opts []flagutil.ParseOption,
	rest func() []string,
) {
	posixParser := &pargs.Parser{
		Args:      args,
		Shorthand: true,
		ShorthandFunc: func(s string) string {
			switch s {
			case "project":
				return "p"
			case "commits":
				return "c"
			case "debug":
				return "d"
			case "mode":
				return "m"
			}
			return ""
		},
	}
	envParser := &env.Parser{
		Prefix:       "RW_",
		SetSeparator: "_",
	}
	fileParser := &file.Parser{
		Lookup: file.LookupFlag(fs, "config"),
		Syntax: &yaml.Syntax{},
	}
	opts = []flagutil.ParseOption{
		flagutil.WithParser(posixParser),
		flagutil.WithParser(envParser),
		flagutil.WithParser(fileParser),
	}
	return opts, posixParser.NonOptionArgs
}

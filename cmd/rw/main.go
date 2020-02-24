package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path"
	"syscall"

	"github.com/gobwas/flagutil"
	"github.com/gobwas/flagutil/parse/env"
	"github.com/gobwas/flagutil/parse/file"
	"github.com/gobwas/flagutil/parse/file/toml"
	"github.com/gobwas/flagutil/parse/pargs"
	"github.com/gobwas/rw"
	"github.com/gobwas/rw/github"
)

func main() {
	log.SetFlags(log.Lshortfile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var force bool
	trap(syscall.SIGINT, func() {
		if force {
			os.Exit(1)
		}
		force = true
		cancel()
	})

	var (
		debug bool
	)
	fs := flag.NewFlagSet("review", flag.PanicOnError)
	fs.BoolVar(&debug,
		"debug", false,
		"print debug logs",
	)

	var setupClient func() *github.Client
	flagutil.Subset(fs, "github", func(fs *flag.FlagSet) {
		setupClient = github.SetupClient(fs)
	})

	var setupReview func() *rw.Review
	flagutil.Subset(fs, "review", func(fs *flag.FlagSet) {
		setupReview = rw.SetupReview(fs)
	})

	flagutil.Parse(ctx, fs,
		flagutil.WithParser(&pargs.Parser{
			Args: os.Args[1:],
		}),
		flagutil.WithParser(&env.Parser{
			Prefix:       "RW_",
			SetSeparator: "_",
		}),
		flagutil.WithParser(&file.Parser{
			Lookup: file.MultiLookup{
				file.LookupFlag(fs, "c"),
				file.PathLookup(path.Join(homedir(), ".rw/config.toml")),
			},
			Syntax: &toml.Syntax{},
		}),
	)
	if !debug {
		log.SetOutput(ioutil.Discard)
	}

	client := setupClient()
	if err := client.Init(ctx); err != nil {
		log.Fatal(err)
	}

	r := setupReview()
	r.Provider = client
	if err := r.Start(ctx); err != nil {
		fmt.Printf("review failed: %v\n", err)
		os.Exit(1)
	}
}

func trap(s os.Signal, fn func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, s)
	go func() {
		for range ch {
			fn()
		}
	}()
}

func homedir() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.HomeDir
}

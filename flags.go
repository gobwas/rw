package rw

import (
	"flag"

	"github.com/gobwas/flagutil"
)

func DefineFlags(r *Review, fs *flag.FlagSet) {
	fs.IntVar(&r.ContextBefore,
		"before", DefaultContext,
		"context to add before changed line(s) for quick mode",
	)
	fs.IntVar(&r.ContextAfter,
		"after", DefaultContext,
		"context to add after changed line(s) for quick mode",
	)
	fs.BoolVar(&r.Preview,
		"preview", false,
		"preview comments before send",
	)
	fs.BoolVar(&r.Comments,
		"comments", false,
		"annotate changed file with comments from vcs provider",
	)
	fs.Var(&r.Mode,
		"mode",
		"review mode",
	)
	flagutil.Subset(fs, "editor", func(fs *flag.FlagSet) {
		fs.StringVar(&r.Editor,
			"name", DefaultEditor,
			"a command-line tool to edit review",
		)
		// TODO: generate supported variables depending on reviewInfo struct.
		fs.Var(&r.EditorArgs,
			"args",
			"args to be passed to the editor; may support variables: Head, Base",
		)
	})
	flagutil.Subset(fs, "finder", func(fs *flag.FlagSet) {
		fs.StringVar(&r.Finder,
			"name", "",
			"a command-line finder to be used for prompts",
		)
		// TODO: generate supported variables depending on finderInfo struct.
		fs.Var(&r.FinderArgs,
			"args",
			"args to be passed to the finder",
		)
	})
}

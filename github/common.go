package github

import (
	"context"
	"fmt"
	"log"
	"math/rand"

	"github.com/gobwas/rw/git"
)

func checkout(ctx context.Context, git *git.Repository, hash string) (root string, cleanup func() error, err error) {
	origBranch, err := git.CurrentBranch(ctx)
	if err != nil {
		return "", nil, err
	}
	if origBranch == "" {
		return "", nil, fmt.Errorf("internal error: checkout: no current branch")
	}
	cleanup = func() error {
		return git.SwitchBranch(ctx, origBranch)
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	clean, err := git.IsClean(ctx)
	if err != nil {
		return "", nil, err
	}
	if !clean {
		return "", nil, fmt.Errorf(
			"github: can't git checkout: working tree is not clean",
		)
	}
	if err := git.SwitchBranch(ctx, hash); err != nil {
		return "", nil, err
	}
	tmpBranch := fmt.Sprintf("review-%x", rand.Int63())
	if err := git.CreateBranch(ctx, tmpBranch); err != nil {
		return "", nil, err
	}
	log.Printf("using %s as root dir for checkout", git.Dir)
	return git.Dir, func() error {
		e0 := git.Restore(context.TODO(), ".")
		e1 := git.Clean(context.TODO())
		e2 := git.SwitchBranch(context.TODO(), origBranch)
		e3 := git.DeleteBranch(context.TODO(), tmpBranch)
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

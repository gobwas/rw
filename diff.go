package rw

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/gobwas/rw/ed"
	"github.com/gobwas/rw/vcs"
)

func (r *Review) reviewDiff(ctx context.Context, review vcs.Review) error {
	files, err := review.ChangedFiles(ctx)
	if err != nil {
		return err
	}
	xs, err := r.selectMultiple(ctx, "Pick files to review:", files)
	if err != nil {
		return err
	}
	files = pick(files, xs...)

	tmp := temp{
		name: "rw",
	}
	for _, file := range files {
		baseSrc, err := review.BaseFile(ctx, file)
		if err != nil {
			return err
		}
		roBase, err := tmp.createFile(baseSrc, "base", file+".ro", 0444)
		if err != nil {
			return err
		}

		headSrc, err := review.HeadFile(ctx, file)
		if err != nil {
			return err
		}
		var (
			roHead *os.File
			rwHead *os.File

			comments []commentBlock
		)
		if r.Comments {
			cs, err := review.FileComments(ctx, file)
			if err != nil {
				return err
			}
			var head *os.File
			head, comments, err = annotate(headSrc, cs)
			if err != nil {
				return err
			}
			roHead, err = tmp.createFile(head, "head", file+".ro", 0444)
			if err != nil {
				return err
			}
			if _, err := head.Seek(0, io.SeekStart); err != nil {
				return err
			}
			rwHead, err = tmp.createFile(head, "head", file+".rw", 0644)
			if err != nil {
				return err
			}
		} else {
			bts, err := ioutil.ReadAll(headSrc)
			if err != nil {
				return err
			}
			roHead, err = tmp.createFile(bytes.NewReader(bts), "head", file+".ro", 0444)
			if err != nil {
				return err
			}
			rwHead, err = tmp.createFile(bytes.NewReader(bts), "head", file+".rw", 0644)
			if err != nil {
				return err
			}
		}

		//err = launch(ctx, "code", "--wait", "--diff", headFileEdit, baseFile)
		err = r.launchEditor(ctx, reviewInfo{
			HeadFile: fileInfo{Name: rwHead.Name()},
			BaseFile: fileInfo{Name: roBase.Name()},
		})
		if err != nil {
			return err
		}

		// NOTE: there is case when user adds two lines right before and right
		// after single comments block. In that case will be produced two edits
		// with same line range. For now it's okay, but maybe it might be glued.
		var edits []ed.Command
		err = diff(ctx, roHead.Name(), rwHead.Name(), func(cmd ed.Command) {
			if comments != nil {
				applyEdit(comments, cmd, appendEditFunc(&edits))
			} else {
				edits = appendEdit(edits, cmd)
			}
		})
		if err != nil {
			return err
		}
		if r.Preview {
			// TODO If preview: create source file again from head; annotateIt with fake comments and show.
			// Fake comments might be created with provider.Preview(edit) -> vcs.Comment.
		}
		for _, cmd := range edits {
			log.Println("applying edit", cmd.Start, cmd.End)
			log.Println(string(cmd.Text))
			if err := review.Edit(ctx, file, cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

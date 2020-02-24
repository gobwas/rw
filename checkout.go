package rw

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gobwas/rw/vcs"
)

type file struct {
	name string
	line int
}

func checkoutFile(name string) file {
	return file{name: name}
}
func checkoutFileLine(name string, line int) file {
	return file{name: name, line: line}
}

func (r *Review) checkout(ctx context.Context, review vcs.Review, editFiles ...file) (err error) {
	headDir, cleanup, err := review.Checkout(ctx)
	if err != nil {
		return err
	}
	defer func() {
		e := cleanup()
		if e != nil {
			log.Printf("warning: cleanup error: %v", err)
		}
		if err == nil {
			err = e
		}
	}()
	changedFiles, err := review.ChangedFiles(ctx)
	if err != nil {
		return err
	}
	tmp := temp{
		name: "rw",
	}
	baseDir, err := tmp.dir()
	if err != nil {
		return err
	}
	headToBase := make(map[string]string)
	for _, file := range changedFiles {
		log.Printf("processing base file for %q", file)
		base, err := review.BaseFile(ctx, file)
		if err != nil {
			return err
		}
		roBase, err := tmp.createFile(base, "base", file, 0444)
		if err != nil {
			return err
		}
		headFile := filepath.Join(headDir, file)
		// Need to fixup deleted files here.
		// FIXME: filepath.Join; review.SourceFile(ctx, file)?
		if _, err := os.Stat(headFile); os.IsNotExist(err) {
			// Need to restore full dir path.
			if err := os.MkdirAll(filepath.Dir(headFile), 0755); err != nil {
				return err
			}
			headFile += ".deleted"
			f, err := os.Create(headFile)
			if err != nil {
				return err
			}
			f.Close()
			log.Printf("touched file %s", f.Name())
		}
		//if r.Comments {
		//	head, err := review.HeadFile(ctx, file)
		//	if err != nil {
		//		return err
		//	}
		//	cs, err := review.FileComments(ctx, file)
		//	if err != nil {
		//		return err
		//	}
		//	f, _, err := annotate(head, cs)
		//	if err != nil {
		//		return err
		//	}
		//	if err := os.Rename(f.Name(), headFile); err != nil {
		//		return err
		//	}
		//	if os.Remove(f.Name()); err != nil {
		//		log.Printf("warning: remove temp file error: %v", err)
		//	}
		//}
		headToBase[headFile] = roBase.Name()
	}
	// FIXME: probably this have to be moved to github impl bc of knowledge about .git?
	// FIXME: do `ln -s` stuff only optionally
	//err = filepath.WalkDir(headDir, func(p string, d fs.DirEntry, e error) error {
	//	if e != nil {
	//		return e
	//	}
	//	if p == headDir {
	//		return nil
	//	}
	//	switch d.Name() {
	//	case
	//		".git",
	//		".github":
	//		return filepath.SkipDir
	//	}
	//	normPath := strings.TrimPrefix(p, headDir)
	//	if d.IsDir() {
	//		// Happy path -- we can symlink the whole dir.
	//		n := filepath.Join(tmpDir, "base", normPath)
	//		log.Printf("ln -s %q %q", p, n)
	//		err := os.Symlink(p, n)
	//		if os.IsExist(err) {
	//			log.Printf("%q already exists; going further", n)
	//			return nil
	//		}
	//		if err != nil {
	//			return err
	//		}
	//		return filepath.SkipDir
	//	}
	//	if headToBase[p] == "" {
	//		// Path is a file and it wasn't changed, so symlink it.
	//		n := filepath.Join(tmpDir, "base", normPath)
	//		log.Printf("ln -s %q %q", p, n)
	//		err := os.Symlink(p, n)
	//		if err != nil {
	//			return err
	//		}
	//	}
	//	return nil
	//})
	//if err != nil {
	//	return err
	//}

	//var cmds []string
	for _, file := range editFiles {
		headFile := filepath.Join(headDir, file.name)
		baseFile := headToBase[headFile]
		if baseFile == "" {
			return fmt.Errorf(
				"internal error: asked to edit non-changed file %q",
				file,
			)
		}
		err := r.launchEditor(ctx, reviewInfo{
			HeadDir: headDir,
			BaseDir: baseDir,

			HeadFile: fileInfo{
				Name: headFile,
				Line: file.line,
			},
			BaseFile: fileInfo{
				Name: baseFile,
			},

			PathSeparator: string(filepath.Separator),
		})
		if err != nil {
			return err
		}
	}
	//return launch(ctx, "nvim", append([]string{
	//	"-c", fmt.Sprintf("cd %s", headDir),
	//	"-c", "set background=light",
	//	"-c", strings.Join([]string{
	//		"execute \":function RWDiffMaybe() \n ",
	//		fmt.Sprintf(`
	//			let filename = join(['%s','base',bufname('%%')], '%s')
	//			if filereadable(bufname('%%'))
	//				execute 'only'
	//			endif
	//			if filereadable(filename)
	//				execute 'vert diffsplit ' .. filename
	//			endif
	//		`, baseDir, string(filepath.Separator)),
	//		" \n endfunction\"",
	//	}, ""),
	//	"-c", "augroup rw_runtime | autocmd VimEnter,BufWinEnter * | call RWDiffMaybe() | augroup END",
	//}, cmds...,
	//)...)
	//return launch(ctx, "vim", append([]string{
	//	"-c", "set background=dark",
	//	"-c", "let g:gitgutter_diff_base='" + review.BaseName() + "'",
	//	"-c", ":GitGutter",
	//	"-c", "tabnew ",
	//	"-p"}, files...,
	//)...)
	//for _, file := range files {
	//	log.Println("LOCAL", file, "REMOTE", review.BaseFileName(file))
	//	err = r.launchEditor(ctx, reviewInfo{
	//		Head:  file,
	//		Base: review.BaseFileName(file),
	//	})
	//if err != nil {
	//	return err
	//}
	//}
	return nil
}

func (r *Review) reviewCheckout(ctx context.Context, review vcs.Review) (err error) {
	files, err := review.ChangedFiles(ctx)
	if err != nil {
		return err
	}
	xs, err := r.selectMultiple(ctx, "Pick files to review:", files)
	if err != nil {
		return err
	}
	files = pick(files, xs...)
	var fs []file
	for _, name := range files {
		fs = append(fs, checkoutFile(name))
	}
	return r.checkout(ctx, review, fs...)
}

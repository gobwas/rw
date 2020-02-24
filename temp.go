package rw

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type temp struct {
	name string

	path string
	err  error
}

func (r *temp) init() {
	if r.err != nil {
		return
	}
	if r.path != "" {
		return
	}
	r.path, r.err = ioutil.TempDir("", r.name)
}

func (r *temp) dir() (string, error) {
	r.init()
	return r.path, r.err
}

func (r *temp) createFile(src io.Reader, prefix, file string, mode os.FileMode) (f *os.File, err error) {
	r.init()
	if r.err != nil {
		return f, r.err
	}
	filename := filepath.Join(r.path, prefix, file)
	defer func() {
		log.Printf(
			"temp: create temp file: %s (err %v)",
			filename, err,
		)
	}()
	r.err = mkdirp(r.path, strings.TrimPrefix(path.Dir(filename), r.path))
	if r.err != nil {
		return nil, r.err
	}
	f, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		r.err = err
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(filename)
		}
	}()

	_, r.err = io.Copy(f, src)
	if r.err != nil {
		return nil, r.err
	}

	return f, nil
}

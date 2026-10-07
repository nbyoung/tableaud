package export

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// checkOut applies the rule of the output directory: it is absent, with an
// existing parent, or it is an empty directory. Anything else is refused, and a
// parent that is absent is a failure to write. It reports whether the
// directory exists.
func checkOut(dir string) (exists bool, err error) {
	fi, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		parent := filepath.Dir(dir)
		if pi, perr := os.Stat(parent); perr != nil || !pi.IsDir() {
			return false, fmt.Errorf("the parent of the output directory, %s, is not a directory", parent)
		}
		return false, nil
	case err != nil:
		return false, err
	case !fi.IsDir():
		return false, &refusedError{fmt.Sprintf("the output %s is not a directory", dir)}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true, err
	}
	if len(entries) > 0 {
		return true, &refusedError{fmt.Sprintf("the output directory %s is not empty", dir)}
	}
	return true, nil
}

// write puts the bundle in dir, which is absent or empty, and nowhere else. It
// creates dir when absent and a subdirectory when a file needs one, writes each
// file once in the order of its path with mode 0644, and writes the manifest
// last, so that a directory with a manifest holds a whole bundle. It makes no
// temporary file. When a write fails it removes what it wrote and the
// directories it made, and returns the error.
func write(dir string, files map[string][]byte, manifest []byte) error {
	exists, err := checkOut(dir)
	if err != nil {
		return err
	}
	var written, made []string
	undo := func(err error) error {
		for _, f := range slices.Backward(written) {
			_ = os.Remove(f)
		}
		for _, d := range slices.Backward(made) {
			_ = os.Remove(d)
		}
		return err
	}
	if !exists {
		if err := os.Mkdir(dir, 0o755); err != nil {
			return err
		}
		made = append(made, dir)
	}
	names := make([]string, 0, len(files)+1)
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	put := func(name string, data []byte) error {
		if !fs.ValidPath(name) || name == "." {
			return fmt.Errorf("the bundle path %q leaves the output directory", name)
		}
		// Make each directory of the path that does not exist yet.
		if d := path.Dir(name); d != "." {
			cur := dir
			for _, part := range strings.Split(d, "/") {
				cur = filepath.Join(cur, part)
				switch err := os.Mkdir(cur, 0o755); {
				case err == nil:
					made = append(made, cur)
				case !errors.Is(err, fs.ErrExist):
					return err
				}
			}
		}
		full := filepath.Join(dir, filepath.FromSlash(name))
		f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		written = append(written, full)
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Chmod(full, 0o644)
	}
	for _, n := range names {
		if err := put(n, files[n]); err != nil {
			return undo(err)
		}
	}
	if err := put(manifestPath, manifest); err != nil {
		return undo(err)
	}
	return nil
}

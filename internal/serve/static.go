package serve

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/nbyoung/tableaud/internal/web"
)

// asset is one embedded file with its tag, computed at start.
type asset struct {
	data []byte
	etag string
}

// loadAssets reads every file of web.Static, keyed by its name under static/.
func loadAssets() map[string]asset {
	assets := map[string]asset{}
	_ = fs.WalkDir(web.Static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(web.Static, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		assets[strings.TrimPrefix(p, "static/")] = asset{data: b, etag: `"` + hex.EncodeToString(sum[:]) + `"`}
		return nil
	})
	return assets
}

// serveStatic answers /static/NAME with the embedded file, its tag and
// no-cache, so that a new binary's assets replace the old at the next request.
// A directory and a name with no file answer false: the caller sends the 404.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request, name string) bool {
	a, ok := s.assets[name]
	if !ok {
		return false
	}
	h := w.Header()
	h.Set("ETag", a.etag)
	h.Set("Cache-Control", "no-cache")
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		h.Set("Content-Type", ct)
	}
	http.ServeContent(w, r, name, zeroTime, bytes.NewReader(a.data))
	return true
}

package serve

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// probeModel is a view model with one part, "probe", and one key, "known". The
// part's data is a page, which the probe template of TestMain reads.
type probeModel struct{}

func (probeModel) Part(name, key string) (any, bool) {
	return web.Page{Params: web.Params{Task: "a1c0"}}, name == "probe" && key == "known"
}

// TestRespondNoPart checks that a model that knows no such key answers 404
// with the error page and logs nothing, and that a key it knows answers the
// part. No adapter reaches the server yet, so the test hands respond a page
// with a model.
func TestRespondNoPart(t *testing.T) {
	v, _ := web.Lookup("gates") // TestMain gives it the probe part and its template
	var logs bytes.Buffer
	s := New(Config{Log: log.New(&logs, "", 0)})
	respond := func(key string) *httptest.ResponseRecorder {
		pg := s.page(v, web.NewParams(), "")
		pg.Params.Part, pg.Params.Key = "probe", key
		pg.Body = probeModel{}
		pg.Live = &web.Live{URL: "/gates", Every: "2s", Headers: "{}"}
		w := httptest.NewRecorder()
		s.respond(w, httptest.NewRequest(http.MethodGet, "/gates?part=probe:"+key, nil), http.StatusOK, web.Part, pg, `"tag"`)
		return w
	}

	if w := respond("known"); w.Code != http.StatusOK || w.Body.String() != `<p class="probe">probe a1c0 </p>` {
		t.Errorf("a known key: %d %q", w.Code, w.Body.String())
	}
	w := respond("nosuch")
	if w.Code != http.StatusNotFound {
		t.Errorf("an unknown key: status %d, want 404", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "<h1>404 Not Found</h1>") || !strings.Contains(body, "no such part") {
		t.Errorf("an unknown key: the body is not the 404 page:\n%s", body)
	}
	if w.Header().Get("ETag") != "" {
		t.Error("the 404 page carries a tag")
	}
	if logs.Len() > 0 {
		t.Errorf("an unknown key logs %q", logs.String())
	}
}

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWhichVersionIsNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.7.0", "0.6.9", true},
		{"0.7.0-beta.1", "0.6.1-beta.1", true},
		{"0.7.0-beta.2", "0.7.0-beta.1", true},
		{"0.7.0-beta.10", "0.7.0-beta.9", true},
		{"0.7.0", "0.7.0-beta.9", true},
		{"0.7.0-beta.1", "0.7.0", false},
		{"0.7.0", "0.7.0", false},
		{"1.0.0", "0.99.99", true},
		{"0.7.0", "dev", false},
		{"nonsense", "0.1.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

// The newest version is picked from what GitHub lists, whatever the order, drafts and tags
// that are not versions left out.
func TestTheNewestVersionListed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v0.6.1-beta.1","html_url":"a"},{"tag_name":"v0.7.0-beta.1","html_url":"b"},
			{"tag_name":"v0.9.0","html_url":"c","draft":true},{"tag_name":"nightly","html_url":"d"},{"tag_name":"v0.6.0-beta.1","html_url":"e"}]`))
	}))
	defer server.Close()
	version, page, err := newest(server.URL)
	if err != nil || version != "0.7.0-beta.1" || page != "b" {
		t.Errorf("got %q at %q, %v", version, page, err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer down.Close()
	if _, _, err := newest(down.URL); err == nil {
		t.Error("a server that fails gave no error")
	}
}

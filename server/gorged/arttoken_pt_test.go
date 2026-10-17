package gorged

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestArtTokenKeyIgnoresCurrentPT pins the token-art cache key on the token's
// PRINTED P/T (the wire's ppt fact), never its CURRENT, counter-modified P/T
// (pt). A 1/1 Soldier pumped to 2/2 must resolve to the same blob key as its
// un-pumped sibling — one search, one entry — while a token whose PRINTED P/T
// really differs still gets its own.
func TestArtTokenKeyIgnoresCurrentPT(t *testing.T) {
	ac, hits := tokenArtFixture(t, map[string]string{"Soldier": "Soldier"})

	blob := func(query string) string {
		t.Helper()
		w := httptest.NewRecorder()
		ac.token(w, httptest.NewRequest(http.MethodGet, "/art/token?"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("token lookup %q: want 200, got %d: %s", query, w.Code, w.Body.String())
		}
		var body struct {
			ImageURIs struct {
				Normal string `json:"normal"`
			} `json:"image_uris"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.ImageURIs.Normal == "" {
			t.Fatalf("token lookup %q: empty blob path", query)
		}
		return body.ImageURIs.Normal
	}

	// Precondition: both requests name the same token family and carry the
	// same PRINTED P/T (ppt); only the CURRENT P/T (pt) differs — the exact
	// shape a pumped and an un-pumped sibling put on the wire.
	unpumped := blob("exact=" + url.QueryEscape("Soldier Token") + "&types=Creature+Soldier&pt=1%2F1&ppt=1%2F1")
	pumped := blob("exact=" + url.QueryEscape("Soldier Token") + "&types=Creature+Soldier&pt=2%2F2&ppt=1%2F1")
	if unpumped != pumped {
		t.Fatalf("pumped and un-pumped Soldier resolved to different blobs: %q vs %q", unpumped, pumped)
	}
	if hits.Load() != 1 {
		t.Fatalf("want one search for the shared key, got %d", hits.Load())
	}
	// Control: a genuinely different PRINTED P/T is a different key, so the
	// fix does not collapse distinct tokens into one entry.
	other := blob("exact=" + url.QueryEscape("Soldier Token") + "&types=Creature+Soldier&pt=2%2F2&ppt=2%2F2")
	if other == pumped {
		t.Fatalf("a different printed P/T shared the pumped token's blob %q", other)
	}
	if hits.Load() != 2 {
		t.Fatalf("want a second search for the distinct printed P/T, got %d", hits.Load())
	}
}

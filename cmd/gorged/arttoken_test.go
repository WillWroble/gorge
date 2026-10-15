package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// tokenArtFixture is artFixture for the token path: an httptest server standing
// in for Scryfall's cards/search endpoint, keyed by the base name the query
// carries, plus ac wired to it via searchBaseURL. Each entry's image URL is
// served from the fixture itself, so a hit's blob bytes identify the card
// that was picked.
func tokenArtFixture(t *testing.T, tokens map[string]string) (*artCache, *atomic.Int32) {
	t.Helper()
	var searchHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/cards/search", func(w http.ResponseWriter, r *http.Request) {
		// unique/order/dir are separate endpoint parameters, not query text:
		// a caller that folds them into q would search for the literal words
		// and miss every token (measured against the real API), so the
		// fixture refuses such a request loudly.
		if r.URL.Query().Get("unique") != "cards" || r.URL.Query().Get("order") != "name" {
			http.Error(w, "bad search params", http.StatusBadRequest)
			return
		}
		searchHits.Add(1)
		q := r.URL.Query().Get("q")
		base := q
		if i := strings.Index(q, `name:"`); i >= 0 {
			rest := q[i+6:]
			if j := strings.Index(rest, `"`); j >= 0 {
				base = rest[:j]
			}
		}
		name, ok := tokens[base]
		if !ok {
			http.NotFound(w, r)
			return
		}
		img := srvURL(t, r) + "/img/" + url.PathEscape(base) + ".jpg"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object":      "list",
			"total_cards": 1,
			"has_more":    false,
			"data": []map[string]any{{
				"name":       name,
				"type_line":  "Token Creature — " + base,
				"power":      "1",
				"toughness":  "1",
				"image_uris": map[string]string{"normal": img},
			}},
		})
	})
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-jpeg-bytes:" + r.URL.Path))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	ac, err := newArtCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	ac.client = srv.Client()
	ac.searchBaseURL = srv.URL + "/cards/search?q="
	return ac, &searchHits
}

func TestArtTokenResolvesBySearchAndCaches(t *testing.T) {
	ac, hits := tokenArtFixture(t, map[string]string{"Human": "Human"})
	w := httptest.NewRecorder()
	ac.token(w, httptest.NewRequest(http.MethodGet,
		"/art/token?exact="+url.QueryEscape("Human Token")+"&types=Creature+Human&pt=1%2F1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("token lookup: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Name      string `json:"name"`
		ImageURIs struct {
			Normal string `json:"normal"`
		} `json:"image_uris"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Human Token" {
		t.Fatalf("name: want the requested display name, got %q", body.Name)
	}
	if !strings.HasPrefix(body.ImageURIs.Normal, "/art/blob/") {
		t.Fatalf("image: want a /art/blob/ path, got %q", body.ImageURIs.Normal)
	}
	// The blob route must serve the art the search picked: the fixture names
	// every image with the base it was found under, so the bytes identify it.
	bw := httptest.NewRecorder()
	rb := httptest.NewRequest(http.MethodGet, body.ImageURIs.Normal, nil)
	rb.SetPathValue("key", strings.TrimPrefix(body.ImageURIs.Normal, "/art/blob/"))
	ac.blob(bw, rb)
	if bw.Code != http.StatusOK {
		t.Fatalf("blob: want 200, got %d", bw.Code)
	}
	if got := bw.Body.String(); got != "fake-jpeg-bytes:/img/Human.jpg" {
		t.Fatalf("blob bytes: want the Human token's image, got %q", got)
	}
	// A second request is a disk hit: no second search.
	w2 := httptest.NewRecorder()
	ac.token(w2, httptest.NewRequest(http.MethodGet,
		"/art/token?exact="+url.QueryEscape("Human Token")+"&types=Creature+Human&pt=1%2F1", nil))
	if w2.Code != http.StatusOK || hits.Load() != 1 {
		t.Fatalf("second lookup: want a cached 200 after 1 search, got %d after %d", w2.Code, hits.Load())
	}
}

func TestArtTokenMissIsCached(t *testing.T) {
	ac, hits := tokenArtFixture(t, map[string]string{})
	w := httptest.NewRecorder()
	ac.token(w, httptest.NewRequest(http.MethodGet, "/art/token?exact=Goblin+Token", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown token: want 404, got %d", w.Code)
	}
	w2 := httptest.NewRecorder()
	ac.token(w2, httptest.NewRequest(http.MethodGet, "/art/token?exact=Goblin+Token", nil))
	if w2.Code != http.StatusNotFound || hits.Load() != 1 {
		t.Fatalf("repeat miss: want a cached 404 after 1 search, got %d after %d", w2.Code, hits.Load())
	}
}

func TestArtTokenKeyDoesNotCollideWithCardKey(t *testing.T) {
	if tokenArtKey("Goblin Token", "Creature Goblin", "1/1") == artKey("Goblin Token") {
		t.Fatal("a token's key shares the card namespace: a same-named real card and a token would poison each other's cache entry")
	}
	// And the two routes really do keep their entries apart on disk, even
	// when the token's display name and a real card's name are the same
	// string: named() for "Human" and token() for "Human Token" may not
	// overwrite one another.
	ac, _ := tokenArtFixture(t, map[string]string{"Human": "Human"})
	nw := httptest.NewRecorder()
	ac.named(nw, httptest.NewRequest(http.MethodGet, "/art/named?exact=Human+Token", nil))
	if nw.Code != http.StatusNotFound {
		t.Fatalf("named lookup of a token display name: want the historical 404, got %d", nw.Code)
	}
	tw := httptest.NewRecorder()
	ac.token(tw, httptest.NewRequest(http.MethodGet,
		"/art/token?exact="+url.QueryEscape("Human Token")+"&types=Creature+Human&pt=1%2F1", nil))
	if tw.Code != http.StatusOK {
		t.Fatalf("token lookup: want 200, got %d", tw.Code)
	}
	if got := tw.Body.String(); strings.Contains(got, artKey("Human Token")) {
		t.Fatalf("token response carries a card-namespace key: %s", got)
	}
}

func TestPickTokenCardPrefersNameThenPTRThenTypes(t *testing.T) {
	humanSoldier := &scryNamed{Name: "Human Soldier", Power: "1", Toughness: "1", TypeLine: "Token Creature — Human Soldier"}
	humanKnight := &scryNamed{Name: "Human Knight", Power: "2", Toughness: "2", TypeLine: "Token Creature — Human Knight"}
	goblin := &scryNamed{Name: "Goblin", Power: "1", Toughness: "1", TypeLine: "Token Creature — Goblin"}
	// 1. exact printed name beats everything else.
	if got := pickTokenCard([]*scryNamed{humanSoldier, goblin}, "Goblin Token", "Creature Goblin", "1/1"); got != goblin {
		t.Fatalf("name-equal pick: want the printed-name match, got %q", got.Name)
	}
	// 2. equal names lose to the P/T the wire carries.
	if got := pickTokenCard([]*scryNamed{humanKnight, humanSoldier}, "Human Token", "Creature Human", "1/1"); got != humanSoldier {
		t.Fatalf("pt pick: want the 1/1 Human, got %q", got.Name)
	}
	// 3. the type line breaks a full tie lexicographically, deterministically.
	if got := pickTokenCard([]*scryNamed{humanSoldier, humanKnight}, "Human Token", "Creature Human Knight", ""); got != humanKnight {
		t.Fatalf("type pick: want the Knight's type overlap, got %q", got.Name)
	}
	// 4. a full tie resolves by name, so the same facts always pick the same card.
	if got := pickTokenCard([]*scryNamed{humanKnight, humanSoldier}, "Human Token", "Creature", ""); got != humanKnight {
		t.Fatalf("tie pick: want the lexicographically smaller name, got %q", got.Name)
	}
}

func TestTokenBaseNameStripsSuffix(t *testing.T) {
	for in, want := range map[string]string{
		"Goblin Token":         "Goblin",
		"Phyrexian Germ Token": "Phyrexian Germ",
		"human token":          "human",
		"Emblem — Liliana":     "Emblem — Liliana", // no suffix: searched as printed
		"Token":                "Token",            // too short to carry the suffix
	} {
		if got := tokenBaseName(in); got != want {
			t.Errorf("tokenBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

// srvURL is tokenArtFixture's stand-in for the fixture's own address inside a
// handler: the request's scheme://host, which httptest populates from the
// server the request arrived on.
func srvURL(t *testing.T, r *http.Request) string {
	t.Helper()
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

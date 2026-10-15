package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// tokenArtKeyVersion busts the token-art cache independently of artKeyVersion:
// a token's art is chosen by SEARCH (the rules below), not by a named lookup,
// so the same display name must never share a key with a real card's entry
// even when the two names are identical strings. Bumping this string rotates
// every token key at once, exactly as artKeyVersion does for card keys.
const tokenArtKeyVersion = "gorge-art-token-v1"

// scryfallSearchURL is the production cards/search endpoint; tests point
// searchBaseURL at an httptest fixture instead.
const scryfallSearchURL = "https://api.scryfall.com/cards/search?q="

// tokenArtKey derives the cache key for a token's art from the wire facts the
// client sends — display name, type line, printed P/T. Hashing all three (not
// just the name) means two same-named tokens a P/T difference distinguishes
// get their own entries, and the token version prefix keeps every token key
// disjoint from the card keys artKey derives from the same string.
func tokenArtKey(name, types, pt string) string {
	sum := sha256.Sum256([]byte(tokenArtKeyVersion + "\x00" + name + "\x00" + types + "\x00" + pt))
	return hex.EncodeToString(sum[:])
}

// tokenBaseName strips the " Token" suffix Forge's token scripts append to
// every token's display name ("Goblin Token" -> "Goblin"), which is the word
// a token card on Scryfall is actually printed under. A name that carries no
// suffix (an Emblem, a Face-down Creature) is searched as printed; the search
// simply misses and the route records a miss, the same answer today's named
// path gives.
func tokenBaseName(name string) string {
	n := strings.TrimSpace(name)
	if len(n) >= 6 && strings.EqualFold(n[len(n)-6:], " token") {
		n = strings.TrimSpace(n[:len(n)-6])
	}
	return n
}

// token answers GET /art/token?exact=<name>&types=<typeline>&pt=<p/t> with
// the same response shape named() serves, but the art is resolved by a
// cards/search lookup (t:token name:"<base>") instead of a named lookup,
// because Scryfall's named endpoint has never heard of a token's display
// name ("Goblin Token") — every token renders blank today. The client sends
// the token's type line and P/T alongside the name (CardView.is_token rides
// the view), and they feed the deterministic pick in pickTokenCard. The
// cache, the miss records, the single-flight join, the 429 backoff and the
// blob route are all the card path's own machinery, reused unchanged.
func (a *artCache) token(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("exact"))
	if name == "" {
		http.Error(w, "exact is required", http.StatusBadRequest)
		return
	}
	types := strings.TrimSpace(r.URL.Query().Get("types"))
	pt := strings.TrimSpace(r.URL.Query().Get("pt"))
	key := tokenArtKey(name, types, pt)
	hit, err := a.boundedWait(r.Context(), func(c context.Context) (bool, error) {
		return a.ensureToken(c, key, name, types, pt)
	})
	if errors.Is(err, errArtPending) {
		http.Error(w, "card art is still being fetched from the upstream catalog; retry shortly", http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !hit {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Name      string `json:"name"`
		ImageURIs struct {
			Normal string `json:"normal"`
		} `json:"image_uris"`
	}{
		Name: name,
		ImageURIs: struct {
			Normal string `json:"normal"`
		}{Normal: "/art/blob/" + key + ".jpg"},
	})
}

// ensureToken is ensure() over the token key: disk first (art or a recorded
// miss), then one shared fetch. It joins the same single-flight map, so a
// burst of identical token names costs one search.
func (a *artCache) ensureToken(ctx context.Context, key, name, types, pt string) (bool, error) {
	return a.singleFlight(ctx, key, func() (bool, bool) {
		if fileExists(a.jpgPath(key)) {
			return true, true
		}
		if fileExists(a.missPath(key)) {
			return false, true
		}
		return false, false
	}, func() (bool, error) {
		select {
		case a.sem <- struct{}{}:
		case <-ctx.Done():
			return false, ctx.Err()
		}
		defer func() { <-a.sem }()
		card, known, err := a.lookupTokenSearch(ctx, tokenBaseName(name))
		if err != nil {
			return false, err
		}
		if !known {
			return false, a.writeMiss(key)
		}
		pick := pickTokenCard(card, name, types, pt)
		if pick == nil || pick.faceImage(tokenBaseName(name)) == "" {
			return false, a.writeMiss(key)
		}
		if err := a.download(ctx, key, pick.faceImage(tokenBaseName(name))); err != nil {
			return false, err
		}
		return true, nil
	})
}

// scryList is the slice of Scryfall's cards/search response the token path
// reads. scryNamed carries every field the pick and the image download need.
type scryList struct {
	TotalCount int          `json:"total_cards"`
	Data       []*scryNamed `json:"data"`
}

// lookupTokenSearch does the one cards/search round trip: a search for the
// token cards printed under the base name, newest-irrelevant, name-ordered.
// The 404/429/retry semantics mirror lookupNamed exactly — a 404 or an empty
// list is a known miss, 429 backs off and retries, anything else is an error
// — and it acquires no semaphore of its own (the caller holds a.sem across
// the whole fetch, including the image download).
func (a *artCache) lookupTokenSearch(ctx context.Context, base string) ([]*scryNamed, bool, error) {
	for attempt := 0; ; attempt++ {
		// Only the q VALUE is escaped here: unique/order/dir are separate
		// query parameters of the search endpoint, not part of the query
		// text — folding them into q makes Scryfall search for the literal
		// words "&unique=cards" and answer 404 for every token.
		q := fmt.Sprintf("t:token name:%q", base)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			a.searchBaseURL+url.QueryEscape(q)+"&unique=cards&order=name&dir=asc", nil)
		if err != nil {
			return nil, false, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", artUserAgent)
		resp, err := a.doScryfall(req)
		if err != nil {
			return nil, false, err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			more, rerr := a.retry429(ctx, resp, "token search "+base, attempt)
			if rerr != nil {
				return nil, false, rerr
			}
			if !more {
				return nil, false, fmt.Errorf("scryfall token search: status 429 after %d retries", attempt)
			}
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return nil, false, nil
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, false, fmt.Errorf("scryfall token search: status %d", resp.StatusCode)
		}
		var list scryList
		derr := json.NewDecoder(resp.Body).Decode(&list)
		_ = resp.Body.Close()
		if derr != nil {
			return nil, false, derr
		}
		if list.TotalCount == 0 || len(list.Data) == 0 {
			return nil, false, nil
		}
		return list.Data, true, nil
	}
}

// pickTokenCard chooses one card from the search results deterministically.
// Scryfall returns every token card whose name mentions the base word, so a
// bare word (Soldier) matches dozens; the pick is scored, not first:
//
//  1. printed name equals the base name — the token card the display name
//     stripped its suffix from (Goblin Token -> Goblin);
//  2. one of the card's faces does (a multi-faced token card);
//  3. the printed P/T equals the token's wire P/T;
//  4. type-line words the token's wire type line shares (creature types).
//
// Ties resolve by the lexicographically smaller printed name and then the
// list order, which the query itself pins with order=name&dir=asc — so the
// same facts pick the same art forever, replay to replay.
func pickTokenCard(cards []*scryNamed, name, types, pt string) *scryNamed {
	base := tokenBaseName(name)
	var best *scryNamed
	bestScore := []int{-1, 0, 0}
	for _, c := range cards {
		if c == nil {
			continue
		}
		score := []int{0, 0, 0}
		if strings.EqualFold(strings.TrimSpace(c.Name), base) {
			score[0] = 4
		} else if c.faceFor(base) != nil {
			score[0] = 3
		}
		if pt != "" && c.Power != "" && c.Toughness != "" &&
			strings.TrimSpace(c.Power)+"/"+strings.TrimSpace(c.Toughness) == pt {
			score[1] = 2
		}
		score[2] = typeWordOverlap(c.TypeLine, types)
		if score[0] > bestScore[0] ||
			(score[0] == bestScore[0] && score[1] > bestScore[1]) ||
			(score[0] == bestScore[0] && score[1] == bestScore[1] && score[2] > bestScore[2]) ||
			(score[0] == bestScore[0] && score[1] == bestScore[1] && score[2] == bestScore[2] &&
				best != nil && strings.TrimSpace(c.Name) < strings.TrimSpace(best.Name)) {
			best, bestScore = c, score
		}
	}
	return best
}

// typeWordOverlap counts the whitespace-split words the two type lines share,
// case-insensitively. It is a tiebreaker between token cards of the same
// printed name family (a 1/1 Human Soldier vs a 2/2 Human Knight), which is
// exactly the distinction the wire's own type line carries.
func typeWordOverlap(typeLine, wireTypes string) int {
	if typeLine == "" || wireTypes == "" {
		return 0
	}
	words := map[string]bool{}
	for _, w := range strings.Fields(typeLine) {
		words[strings.ToLower(w)] = true
	}
	n := 0
	for _, w := range strings.Fields(wireTypes) {
		if words[strings.ToLower(w)] {
			n++
		}
	}
	return n
}

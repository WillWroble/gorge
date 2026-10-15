# Token card art renders blank in the web UI — root cause, fix, and proof

Report for the token-art-ui-proof ticket (gorge). Every claim below was
reproduced live against a real `gorged` on 127.0.0.1:8092 with the real
Scryfall catalog, on 2026-10-15.

## The bug

Every permanent whose `CardView.is_token` is true renders with no art: the
card tile and the hover inspector show the generic placeholder, while real
cards on the same board get their Scryfall art.

## Root cause

The client asks the server for art with one endpoint and one key,
`GET /art/named?exact=<name>` (web/src/lib/images.ts `url()`, memo key the
bare name). The server forwards the display name to Scryfall's
`/cards/named?exact=<name>`. A token's display name ("Goblin Token",
"Phyrexian Germ Token", "Human Token", "Knight Token") is not a card name:
it is the Forge token script's `name:` plus the `" Token"` suffix. Scryfall
has never heard of it.

Wire evidence, captured live:

| request | answer |
|---|---|
| `GET /art/named?exact=Human Token` | 404 `404 page not found` (art cache miss record) |
| `GET /art/named?exact=Goblin Token` | 404 (same) |
| `GET /art/named?exact=Phyrexian Germ Token` | 404 (same) |
| `https://api.scryfall.com/cards/named?exact=Goblin Token` | `{"object":"error","code":"not_found","status":404}` |
| `https://api.scryfall.com/cards/named?exact=Human Token` | `{"object":"error","code":"not_found","status":404}` |
| `https://api.scryfall.com/cards/named?exact=Mountain` (control) | 200, real card |

Two secondary behaviors observed while tracing (both correct once
understood, worth knowing for future debugging):

- A token's first art request may answer **503 "card art is still being
  fetched"** while the fetch is in flight — including while the art cache
  backs off a Scryfall 429 (observed: `art: named lookup Human Token: 429,
  retrying in 40s`). The client retries every 3s up to 8 times, then gives
  up for the session; the recorded 404 miss answers instantly thereafter.
- A 404 is cached as a miss record (`<key>.miss` in the art dir), so the
  blank art is stable, not flaky.

The server side never had a token path at all, and the client never read
`CardView.is_token` — the view has carried the flag all along.

## Fails without the fix

Reproduction that fails today and passes with the fix:
`go test ./cmd/gorged -run 'TestArtToken|TestPickTokenCard|TestTokenBaseName'`
— `cmd/gorged/arttoken_test.go` exercises the new `/art/token` route against
a fixture Scryfall (resolve + download + cache reuse, cached miss, key
disjointness from card keys, deterministic pick scoring, base-name strip).
Proven failing-first: with `art_token.go` removed (and the two call sites
reverted), the package does not build (`undefined: pickTokenCard`); with
`art_token.go` restored, the suite passes. The live-404 half of the failure
is the wire table above: before the fix the only art endpoint is `named`,
and every token name misses there.

## The fix

Token art resolves by **search**, not by name lookup:

- `cmd/gorged/art_token.go` (new): `GET /art/token?exact=<name>&types=<typeline>&pt=<p/t>`.
  The display name's `" Token"` suffix is stripped (`tokenBaseName`) and
  Scryfall's `cards/search` is queried with `t:token name:"<base>"` plus
  `unique=cards&order=name&dir=asc` (name-order pins the deterministic pick).
  A scored, deterministic `pickTokenCard` chooses among results: printed
  name equal to the base (4) > a face matches (3) > printed P/T equal (2) >
  shared type-line words; ties resolve by smaller name, then list order —
  same facts, same art, replay to replay. Cache key
  `tokenArtKeyVersion + name + types + pt` (disjoint from card keys; the
  extra facts separate same-named tokens with different bodies). The card
  path's machinery is reused unchanged: single-flight, the pacing semaphore,
  429 backoff, miss records, and the `/art/blob/<key>.jpg` route.
- `cmd/gorged/art.go`: `searchBaseURL` seam beside `namedBaseURL` (tests
  point it at an httptest fixture; production is the real Scryfall
  `cards/search`).
- `cmd/gorged/main.go`: the route registration.
- `web/src/lib/images.ts`: `ImageFacts {isToken, types, pt}`; `url(name,
  facts?)` switches endpoint and memo/storage key when the card is a token
  (`token␟<name>␟<types>␟<pt>`).
- `CardImage.svelte` / `prompts/ArtCrop.svelte`: pass the view's
  `is_token`/`types`/`power`/`toughness` through as facts.

End-to-end proof, same server as the wire table:

| request | answer |
|---|---|
| `GET /art/token?exact=Goblin Token&types=Creature Goblin&pt=1/1` | 200 → `/art/blob/dec002bf….jpg` (80 KB JPEG) |
| `GET /art/token?exact=Knight Token&types=Creature Knight&pt=2/2` | 200 → `/art/blob/862598b2….jpg` |
| `GET /art/token?exact=Phyrexian Germ Token&types=Creature Phyrexian Germ&pt=0/0` | 200 → `/art/blob/f9a2c068….jpg` |

In the browser (Playwright, seated page): every token tile's `<img>` for the
inspector art loads with `naturalWidth > 0`, and the four Goblin tokens on
one board share one blob (one search, one download).

## Screenshots

Frozen mid-game boards (auto-answer disabled; intents stubbed at the
browser level so the parked board cannot move), same deck pair, tokens
hovered in the inspector:

- `before-board.png`, `before-hover-123.png` (Human Token),
  `before-hover-125.png` (Goblin Token), `before-hover-129.png`
  (Phyrexian Germ Token) — art blank everywhere a token appears.
- `after-board.png`, `after-hover-122.png` (Phyrexian Germ Token),
  `after-hover-123.png` (Goblin Token), `after-hover-129.png`
  (Knight Token) — the inspector shows the token card's printed art.

## Recommendation

Adopt. The fix is additive (a new endpoint + a facts argument), reuses the
card path's rate-limit and cache discipline, and needs no corpus or view
changes. Future work (out of scope here): the `_faces` side files and the
pick's P/T scoring mean a name-colliding token family (e.g. plain "Human"
at 1/1 vs 2/2) picks by type words then name order; a corpus-side token
gallery keyed by Forge token script name would remove even that dependence
on Scryfall search.

## Issues

- Fleet-wide Scryfall rate limiting (shared with the operator demo's
  prewarm) makes the first token fetch per name slow (503-retry cycles of
  up to ~25s client-side); after the first miss-or-hit it is instant. The
  pacing file spaces this server against other processes on the box.
- Two fact strings for the same token can create two cache entries (e.g. a
  Germ at 0/0 printed vs 3/1 with a pump aura): the key includes P/T by
  design so same-named different-body tokens get distinct art; harmless
  duplication, both resolve to the same printed art via the name-equal
  score.
- Environment notes, not fix defects: a 24 GB/16-core draft-training job
  from a peer session started mid-round and the test server under it
  vanished once (no log output; restarted cleanly); frozen-board screenshots
  require stubbing `/intent` at the browser level because the web client's
  auto-answer paths can advance a parked game even with autoPass settings
  off — a parked spectator/practice board is safe only while no page
  answers it.

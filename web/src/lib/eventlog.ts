/**
 * eventlog.ts reads a COMPLETE event's revealed log (GET /api/events/{id}/log)
 * into what the results screen shows: every seat's final deck and its picks
 * in order. The server redacts other seats' packs, picks and decks until the
 * event is COMPLETE; before then these readers simply find less.
 *
 * The entries are the server's event.Entry JSON. A DeckSaved's deck is
 * build.Deck, {"main": [...], "basics": [W, U, B, R, G]}, with "main"
 * omitted when the deck has no non-basic cards (or the line is redacted).
 */

import { BASIC_NAMES } from './draftcards';

interface CardRefJSON {
  id: string;
  name: string;
  set?: string;
  number?: string;
  rarity?: string;
}

interface EntryJSON {
  kind: string;
  seat?: number;
  round?: number;
  pick_no?: number;
  id?: string;
  rev?: number;
  auto?: boolean;
  cards?: CardRefJSON[];
  deck?: { main?: string[] | null; basics?: number[] };
}

export interface RevealedDeck {
  seat: number;
  rev: number;
  /** card names in deck order, then basics as "N Name" lines */
  cards: string[];
  basics: { name: string; count: number }[];
  total: number;
}

export interface RevealedPick {
  round: number;
  pickNo: number;
  name: string;
  auto: boolean;
}

function entries(raw: unknown[]): EntryJSON[] {
  return raw.filter((e): e is EntryJSON => typeof e === 'object' && e !== null && typeof (e as EntryJSON).kind === 'string');
}

/** cardNames maps every CardRef id a PackOpened revealed to its card name. */
export function cardNames(raw: unknown[]): Map<string, string> {
  const out = new Map<string, string>();
  for (const e of entries(raw)) {
    if (e.kind !== 'PackOpened') continue;
    for (const c of e.cards ?? []) out.set(c.id, c.name);
  }
  return out;
}

/** revealDecks is every seat's LAST saved deck, by seat. Unknown ids read as their id. */
export function revealDecks(raw: unknown[]): RevealedDeck[] {
  const names = cardNames(raw);
  const last = new Map<number, EntryJSON>();
  for (const e of entries(raw)) if (e.kind === 'DeckSaved') last.set(e.seat ?? 0, e);
  return [...last.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([seat, e]) => {
      const main = e.deck?.main ?? [];
      const b = e.deck?.basics ?? [];
      const basics = BASIC_NAMES.map((name, i) => ({ name, count: b[i] ?? 0 })).filter((x) => x.count > 0);
      return {
        seat,
        rev: e.rev ?? 0,
        cards: main.filter((id) => id !== '').map((id) => names.get(id) ?? id),
        basics,
        total: main.length + basics.reduce((a, x) => a + x.count, 0),
      };
    })
    .filter((d) => d.cards.length > 0 || d.basics.length > 0);
}

/** pickHistory is one seat's picks in log order. */
export function pickHistory(raw: unknown[], seat: number): RevealedPick[] {
  const names = cardNames(raw);
  return entries(raw)
    .filter((e) => e.kind === 'Picked' && (e.seat ?? 0) === seat && !!e.id)
    .map((e) => ({ round: e.round ?? 0, pickNo: e.pick_no ?? 0, name: names.get(e.id!) ?? e.id!, auto: !!e.auto }));
}

/** countNames folds a name list into "name × n" rows, first-seen order. */
export function countNames(names: string[]): { name: string; count: number }[] {
  const out: { name: string; count: number }[] = [];
  const at = new Map<string, number>();
  for (const n of names) {
    const i = at.get(n);
    if (i === undefined) {
      at.set(n, out.length);
      out.push({ name: n, count: 1 });
    } else out[i].count++;
  }
  return out;
}

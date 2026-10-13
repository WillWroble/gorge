import { describe, expect, it } from 'vitest';
import { eventSeatFor, forgetEventSeat } from './eventseat';
import { cardNames, countNames, pickHistory, revealDecks } from './eventlog';
import { pct, record, standingRows } from './standings';
import { createRequest, eventPolicies } from './newevent';
import { eventState } from '../test/eventfixtures';
import type { BotPolicyInfo, EventSeries } from '../protocol';

function memStorage(): Storage {
  const m = new Map<string, string>();
  return {
    get length() { return m.size; },
    clear: () => m.clear(),
    getItem: (k) => m.get(k) ?? null,
    key: (i) => [...m.keys()][i] ?? null,
    removeItem: (k) => void m.delete(k),
    setItem: (k, v) => void m.set(k, String(v)),
  };
}

describe('eventSeatFor', () => {
  it('reads ?seat=&token= from the invite link and remembers it for this event only', () => {
    const s = memStorage();
    expect(eventSeatFor('e1', '?seat=0&token=abc123', s)).toEqual({ seat: 0, token: 'abc123' });
    expect(eventSeatFor('e1', '', s)).toEqual({ seat: 0, token: 'abc123' });
    expect(eventSeatFor('e2', '', s)).toBeNull();
    forgetEventSeat('e1', s);
    expect(eventSeatFor('e1', '', s)).toBeNull();
  });
  it('rejects a malformed link like a table join URL does', () => {
    const s = memStorage();
    expect(eventSeatFor('e1', '?seat=x&token=abc', s)).toBeNull();
    expect(eventSeatFor('e1', '?seat=0&token=<script>', s)).toBeNull();
    expect(eventSeatFor('e1', '?token=abc', s)).toBeNull();
  });
});

describe('eventlog reveal', () => {
  // build.Deck carries no JSON tags on the server, so DeckSaved's deck is {Main, Basics}.
  const log = [
    { kind: 'EventCreated', seed: 7, set: 'FDN' },
    { kind: 'PackOpened', round: 1, cards: [{ id: 'c1', name: 'Serra Angel' }, { id: 'c2', name: 'Lightning Bolt' }] },
    { kind: 'PackOpened', round: 1, seat: 1, cards: [{ id: 'c3', name: 'Lightning Bolt' }, { id: 'c4', name: 'Shock' }] },
    { kind: 'Picked', round: 1, pick_no: 1, id: 'c1' },
    { kind: 'Picked', round: 1, pick_no: 1, seat: 1, id: 'c3', auto: true },
    { kind: 'Picked', round: 1, pick_no: 2, id: 'c4' },
    { kind: 'DeckSaved', rev: 1, deck: { Main: ['c1'], Basics: [17, 0, 0, 0, 0] } },
    { kind: 'DeckSaved', rev: 2, deck: { Main: ['c1', 'c4'], Basics: [16, 0, 0, 0, 0] } },
    { kind: 'DeckSaved', seat: 1, rev: 1, deck: { Main: ['c3', 'c2'], Basics: [0, 0, 0, 20, 0] } },
  ];
  it('maps ids to names from the opened packs', () => {
    expect(cardNames(log).get('c4')).toBe('Shock');
  });
  it('reveals each seat\'s LAST saved deck', () => {
    const d = revealDecks(log);
    expect(d.map((x) => x.seat)).toEqual([0, 1]);
    expect(d[0]).toMatchObject({ rev: 2, cards: ['Serra Angel', 'Shock'], basics: [{ name: 'Plains', count: 16 }], total: 18 });
    expect(d[1].cards).toEqual(['Lightning Bolt', 'Lightning Bolt']);
    expect(countNames(d[1].cards)).toEqual([{ name: 'Lightning Bolt', count: 2 }]);
  });
  it('lists one seat\'s picks in order, flagging auto-picks', () => {
    expect(pickHistory(log, 0).map((p) => p.name)).toEqual(['Serra Angel', 'Shock']);
    expect(pickHistory(log, 1)).toEqual([{ round: 1, pickNo: 1, name: 'Lightning Bolt', auto: true }]);
  });
  it('a redacted (pre-COMPLETE) log reveals nothing it hides', () => {
    expect(revealDecks([{ kind: 'DeckSaved', seat: 2, rev: 1, deck: { Main: null, Basics: [0, 0, 0, 0, 0] } }])).toEqual([]);
  });
});

describe('standings rows', () => {
  const series: EventSeries[] = [
    { round: 1, a: 0, b: 3, best_of: 1, state: 'ended', score: [1, 0], winner: 0, table: 't1' },
    { round: 1, a: 1, b: 2, best_of: 1, state: 'ended', score: [0, 0], winner: null, table: 't2' },
    { round: 2, a: 0, b: 2, best_of: 1, state: 'live', score: [0, 0], winner: null, table: 't3' },
  ];
  it('keeps the server order and formats the MTR columns', () => {
    const st = eventState({
      standings: [
        { seat: 0, points: 3, mw: 1, omw: 0.33, gw: 1, ogw: 0.333 },
        { seat: 1, points: 1, mw: 0.33, omw: 0.33, gw: 0.33, ogw: 0.33 },
        { seat: 2, points: 1, mw: 0.33, omw: 0.33, gw: 0.33, ogw: 0.33 },
        { seat: 3, points: 0, mw: 0.33, omw: 1, gw: 0.33, ogw: 1 },
      ],
    });
    const rows = standingRows(st.standings, st.seats, series, 0);
    expect(rows.map((r) => r.seat)).toEqual([0, 1, 2, 3]);
    expect(rows[0]).toMatchObject({ rank: 1, label: 'You', you: true, points: 3, record: '1-0', mw: '100%', omw: '33%', gw: '100%', ogw: '33%' });
    expect(rows[1].record).toBe('0-0-1');
    expect(rows[3]).toMatchObject({ label: 'Seat 4 · Bot 4 (bot)', record: '0-1', omw: '100%' });
  });
  it('pct and record helpers', () => {
    expect(pct(0.3333)).toBe('33%');
    expect(record(2, series)).toBe('0-0-1');
  });
});

describe('create request', () => {
  const p = (name: string, formats: string[]): BotPolicyInfo => ({
    name, label: name, description: '', tier: 'production', strength: [], mean_ms: 0, p95_ms: 0, cost_scope: '', cost_note: '', search: false, formats,
  });
  it('offers only policies that play constructed', () => {
    expect(eventPolicies([p('a', ['constructed', 'commander']), p('b', ['commander'])]).map((x) => x.name)).toEqual(['a']);
  });
  it('seat 1 is the human; an empty policy keeps the server default', () => {
    expect(createRequest('FDN', ['lethal-pressure', '', 'bot'])).toEqual({
      set: 'FDN',
      seats: [{ kind: 'human' }, { kind: 'bot', policy: 'lethal-pressure' }, { kind: 'bot' }, { kind: 'bot', policy: 'bot' }],
    });
  });
});

describe('CardFactsStore', () => {
  it('resolves each name once, and asks again only after a null (the catalog may have been backed off)', async () => {
    const { CardFactsStore } = await import('./cardfacts.svelte');
    const asked: string[] = [];
    let online = false;
    const store = new CardFactsStore(async (n) => {
      asked.push(n);
      return online ? { name: n, mana_cost: '{1}{G}', type_line: 'Creature — Elf' } : null;
    });
    store.want(['Elf']);
    await Promise.resolve();
    await Promise.resolve();
    expect(store.of('Elf').mv).toBeNull();
    online = true;
    store.want(['Elf']);
    await Promise.resolve();
    await Promise.resolve();
    expect(store.of('Elf')).toMatchObject({ mv: 2, colours: ['G'], creature: true });
    store.want(['Elf']);
    expect(asked).toEqual(['Elf', 'Elf']);
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeckBuilder, STEPPER_MAX } from './deckbuild.svelte';
import { eventPool, eventState, fakeFetch, type Recorded } from '../test/eventfixtures';
import type { DeckCheck, DeckRequest } from '../protocol';

afterEach(() => vi.unstubAllGlobals());

// A stand-in for the server's deck check: it echoes the total and answers a
// fixed auto-land split, so a test can tell the client SHOWS what it is
// told rather than computing anything itself.
const checkReply = (r: Recorded): { body: DeckCheck } => {
  const d = r.body as DeckRequest;
  const total = d.main.length + d.basics.reduce((a, n) => a + n, 0);
  return { body: { valid: total >= 40, total, error: total >= 40 ? undefined : `build: deck has ${total} cards, needs at least 40`, auto_lands: [0, 0, 0, 19, 19] } };
};

async function loaded(routes: Parameters<typeof fakeFetch>[0] = {}, pool = eventPool()) {
  const f = fakeFetch({
    'GET /api/events/e1/pool': { body: pool },
    'POST /api/events/e1/deck/check': checkReply,
    ...routes,
  });
  const b = new DeckBuilder('e1', 'tok0');
  await b.load();
  return { b, ...f };
}

const settle = async () => {
  for (let i = 0; i < 20; i++) await Promise.resolve();
};

describe('DeckBuilder', () => {
  it('starts from the server suggestion when nothing is saved, and shows the server check', async () => {
    const { b, calls } = await loaded();
    expect(b.main).toEqual(['c2', 'c3']);
    expect(b.basics).toEqual([0, 0, 0, 8, 9]);
    expect(b.check).toMatchObject({ valid: false, total: 19 });
    expect(b.check?.error).toMatch(/needs at least 40/);
    expect(calls.every((c) => c.auth === 'Bearer tok0')).toBe(true);
  });

  it('starts from the saved deck when there is one', async () => {
    const { b } = await loaded({}, eventPool({ deck: { main: ['c1'], basics: [17, 0, 0, 0, 0] }, rev: 2 }));
    expect(b.main).toEqual(['c1']);
    expect(b.rev).toBe(2);
    expect(b.dirty).toBe(false);
  });

  it('click/tap toggles a card between pool and deck; a drop moves it to the named zone', async () => {
    const { b } = await loaded();
    b.move('c1'); // click: side -> deck
    expect(b.inDeck('c1')).toBe(true);
    b.move('c1'); // click again: deck -> side
    expect(b.inDeck('c1')).toBe(false);
    b.move('c4', 'deck'); // drop on the deck zone
    expect(b.inDeck('c4')).toBe(true);
    b.move('c4', 'deck'); // dropping where it already is changes nothing
    expect(b.main.filter((x) => x === 'c4')).toHaveLength(1);
    b.move('c2', 'side'); // drop on the pool zone
    expect(b.inDeck('c2')).toBe(false);
    expect(b.sideCards.map((c) => c.id)).toContain('c2');
    expect(b.deckCards.map((c) => c.id)).toEqual(['c3', 'c4']);
  });

  it('ignores an id that is not in the pool (a crafted drop)', async () => {
    const { b } = await loaded();
    b.move('c999', 'deck');
    expect(b.main).toEqual(['c2', 'c3']);
  });

  it('every change asks the server again; the steppers clamp at 0 and the hosting bound', async () => {
    const { b, calls } = await loaded();
    const checks = () => calls.filter((c) => c.path.endsWith('/deck/check')).length;
    const before = checks();
    b.step(0, 1);
    b.step(0, -1);
    b.step(0, -1); // already 0: no change, no request
    await settle();
    expect(b.basics[0]).toBe(0);
    expect(checks()).toBe(before + 2);
    for (let i = 0; i < 50; i++) b.step(4, 1);
    expect(b.basics[4]).toBe(STEPPER_MAX);
    await settle();
    expect(b.check?.total).toBe(2 + 8 + STEPPER_MAX);
  });

  it('drops a check answer for an older working copy', async () => {
    let release: (() => void) | null = null;
    let n = 0;
    const { b } = await loaded({
      'POST /api/events/e1/deck/check': (r) => {
        n++;
        if (n === 2) return new Promise((res) => (release = () => res({ body: { valid: false, total: -1, auto_lands: [0, 0, 0, 0, 0] } })));
        return checkReply(r);
      },
    });
    b.step(1, 1); // check #2: held
    b.step(1, 1); // check #3: answers at once
    await settle();
    expect(b.check?.total).toBe(21);
    release!();
    await settle();
    expect(b.check?.total).toBe(21); // the stale answer did not overwrite
  });

  it('Auto takes the server auto_lands for the current main', async () => {
    const { b } = await loaded();
    await b.autoLands();
    expect(b.basics).toEqual([0, 0, 0, 19, 19]);
    expect(b.check).toMatchObject({ valid: true, total: 40 });
  });

  it('Reset restores the server suggestion', async () => {
    const { b } = await loaded();
    b.move('c1', 'deck');
    b.step(0, 3);
    await b.resetToSuggestion();
    expect(b.main).toEqual(['c2', 'c3']);
    expect(b.basics).toEqual([0, 0, 0, 8, 9]);
  });

  it('Ready saves the working copy, then readies exactly the revision the save produced', async () => {
    const { b, calls } = await loaded({
      'POST /api/events/e1/deck': (r) => ({ body: { ...checkReply(r).body, rev: 3 } }),
      'POST /api/events/e1/ready': { body: eventState({ phase: 'BUILD' }) },
    });
    const states: string[] = [];
    b.onstate = (st) => states.push(st.phase);
    await b.autoLands();
    expect(await b.readyUp()).toBe(true);
    expect(states).toEqual(['BUILD']); // the page adopts the state Ready answered with
    const save = calls.find((c) => c.path === '/api/events/e1/deck')!;
    expect(save.body).toEqual({ main: ['c2', 'c3'], basics: [0, 0, 0, 19, 19] });
    expect(calls.find((c) => c.path === '/api/events/e1/ready')?.body).toEqual({ rev: 3 });
    expect(b.rev).toBe(3);
    expect(b.ready).toBe(true);
    expect(b.dirty).toBe(false);
    // Ready again with no change re-readies without saving another revision.
    await b.readyUp();
    expect(calls.filter((c) => c.path === '/api/events/e1/deck')).toHaveLength(1);
  });

  it('shows the server refusal when a save is rejected (422 invalid_deck)', async () => {
    const { b, calls } = await loaded({
      'POST /api/events/e1/deck': { status: 422, body: { code: 'invalid_deck', message: 'eventhost: build: deck has 19 cards, needs at least 40' } },
    });
    expect(await b.readyUp()).toBe(false);
    expect(b.error).toBe('eventhost: build: deck has 19 cards, needs at least 40');
    expect(b.ready).toBe(false);
    expect(calls.some((c) => c.path === '/api/events/e1/ready')).toBe(false);
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import { Sideboard } from './sideboard.svelte';
import { eventPool, eventState, fakeFetch, type Recorded } from '../test/eventfixtures';
import type { DeckCheck, DeckRequest } from '../protocol';

afterEach(() => vi.unstubAllGlobals());

// The server's deck check stand-in: echoes the total, so a test sees the
// client SHOW the answer rather than compute one.
const checkReply = (r: Recorded): { body: DeckCheck } => {
  const d = r.body as DeckRequest;
  const total = d.main.length + d.basics.reduce((a, n) => a + n, 0);
  return { body: { valid: total >= 40, total, error: total >= 40 ? undefined : `build: deck has ${total} cards, needs at least 40`, auto_lands: [0, 0, 0, 19, 19] } };
};

// The deck played last game (EventPool.playing) differs from both the saved
// BUILD deck and the bot suggestion, so a test can tell which one started.
const pool = eventPool({
  deck: { main: ['c1', 'c2'], basics: [19, 0, 0, 19, 0] },
  rev: 3,
  ready: true,
  playing: { main: ['c2', 'c3'], basics: [0, 0, 0, 19, 19] },
  playing_rev: 3,
});

async function loaded(routes: Parameters<typeof fakeFetch>[0] = {}) {
  const f = fakeFetch({
    'GET /api/events/e1/pool': { body: pool },
    'POST /api/events/e1/deck/check': checkReply,
    ...routes,
  });
  const sb = new Sideboard('e1', 'tok0');
  await sb.load();
  return { sb, ...f };
}

describe('Sideboard', () => {
  it('starts the working copy from the deck played last game and shows the server check', async () => {
    const { sb, calls } = await loaded();
    expect(sb.builder.main).toEqual(['c2', 'c3']);
    expect(sb.builder.basics).toEqual([0, 0, 0, 19, 19]);
    expect(sb.changed).toBe(false);
    expect(sb.builder.check).toMatchObject({ valid: true, total: 40 });
    expect(calls.every((c) => c.auth === 'Bearer tok0')).toBe(true);
  });

  it('Keep sends {game, keep: true} and hands the reply to onstate', async () => {
    const reply = eventState({ phase: 'ROUND1', round: 1 });
    const { sb, calls } = await loaded({ 'POST /api/events/e1/sideboard': { body: reply } });
    const onstate = vi.fn();
    sb.onstate = onstate;
    expect(await sb.keep(2)).toBe(true);
    const post = calls.find((c) => c.path === '/api/events/e1/sideboard')!;
    expect(post.body).toEqual({ game: 2, keep: true });
    expect(post.auth).toBe('Bearer tok0');
    expect(onstate).toHaveBeenCalledWith(reply);
    expect(sb.submitted).toBe(true);
  });

  it('a swap submits {game, deck} with the moved ids and the basics', async () => {
    const { sb, calls } = await loaded({ 'POST /api/events/e1/sideboard': { body: eventState() } });
    sb.builder.move('c3', 'side');
    sb.builder.move('c4', 'deck');
    sb.builder.step(4, 1);
    expect(sb.changed).toBe(true);
    expect(await sb.submit(3)).toBe(true);
    const post = calls.find((c) => c.path === '/api/events/e1/sideboard')!;
    expect(post.body).toEqual({ game: 3, deck: { main: ['c2', 'c4'], basics: [0, 0, 0, 19, 20] } });
  });

  it('a 409 refetches the state (onstale) and the pool, and says why', async () => {
    let pools = 0;
    const { sb } = await loaded({
      'GET /api/events/e1/pool': () => (pools++, { body: pool }),
      'POST /api/events/e1/sideboard': { status: 409, body: { code: 'conflict', message: 'eventhost: seat 0 already sideboarded for game 2' } },
    });
    const onstale = vi.fn();
    sb.onstale = onstale;
    sb.builder.move('c3', 'side');
    expect(await sb.submit(2)).toBe(false);
    expect(onstale).toHaveBeenCalledTimes(1);
    expect(pools).toBe(2); // the first load, then the reload after the 409
    expect(sb.builder.main).toEqual(['c2', 'c3']); // the server's copy again
    expect(sb.notice).toMatch(/moved on/);
    expect(sb.error).toBeNull();
    expect(sb.submitted).toBe(false);
  });

  it('a 422 shows the server\'s message and keeps the working copy', async () => {
    const { sb } = await loaded({
      'POST /api/events/e1/sideboard': { status: 422, body: { code: 'invalid_deck', message: 'build: deck has 39 cards, needs at least 40' } },
    });
    sb.builder.move('c3', 'side');
    expect(await sb.submit(2)).toBe(false);
    expect(sb.error).toBe('build: deck has 39 cards, needs at least 40');
    expect(sb.builder.main).toEqual(['c2']);
    expect(sb.submitted).toBe(false);
  });
});

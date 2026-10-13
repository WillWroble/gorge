import { afterEach, describe, expect, it, vi } from 'vitest';
import { EventClient, patchSeries } from './draftevent.svelte';
import { card, draftPack, eventState, fakeFetch } from '../test/eventfixtures';
import type { EventSeries, Frame } from '../protocol';

const seat = { seat: 0, token: 'tok0' };
const frame = (t: string, body: unknown): Frame => ({ v: 1, t, seq: 0, body });

afterEach(() => vi.unstubAllGlobals());

describe('EventClient: load', () => {
  it('fetches the seat state and, while drafting, its pack — with the token as a Bearer header, never in the URL', async () => {
    const { calls } = fakeFetch({
      'GET /api/events/e1': { body: eventState() },
      'GET /api/events/e1/pack': { body: draftPack() },
    });
    const c = new EventClient('e1', seat);
    await c.load();
    expect(c.state?.phase).toBe('DRAFT');
    expect(c.you).toBe(0);
    expect(c.pack?.pick_no).toBe(3);
    expect(calls.map((x) => `${x.method} ${x.path}`)).toEqual(['GET /api/events/e1', 'GET /api/events/e1/pack']);
    for (const x of calls) {
      expect(x.auth).toBe('Bearer tok0');
      expect(x.path).not.toContain('tok0');
    }
  });

  it('a spectator sends no token and asks for no pack', async () => {
    const { calls } = fakeFetch({ 'GET /api/events/e1': { body: eventState({ you: null }) } });
    const c = new EventClient('e1', null);
    await c.load();
    expect(c.you).toBeNull();
    expect(calls).toHaveLength(1);
    expect(calls[0].auth).toBeNull();
  });

  it('marks a missing event (404) instead of erroring', async () => {
    fakeFetch({ 'GET /api/events/e9': { status: 404, body: { code: 'not_found', message: 'no event' } } });
    const c = new EventClient('e9', seat);
    await c.load();
    expect(c.missing).toBe(true);
    expect(c.error).toBeNull();
  });

  it('falls back to spectating when the server refuses the seat token (401)', async () => {
    fakeFetch({
      'GET /api/events/e1': (r) =>
        r.auth ? { status: 401, body: { code: 'unauthorized', message: 'unknown seat token' } } : { body: eventState({ you: null }) },
    });
    const c = new EventClient('e1', seat);
    await c.load();
    expect(c.unauthorised).toBe(true);
    expect(c.token).toBeNull();
    expect(c.state?.you).toBeNull();
  });
});

describe('EventClient: select and pick', () => {
  async function drafting(routes: Parameters<typeof fakeFetch>[0] = {}) {
    const f = fakeFetch({
      'GET /api/events/e1': { body: eventState() },
      'GET /api/events/e1/pack': { body: draftPack() },
      ...routes,
    });
    const c = new EventClient('e1', seat);
    await c.load();
    return { c, ...f };
  }

  it('select marks only a card in the current pack', async () => {
    const { c } = await drafting();
    c.select('c11');
    expect(c.selected).toBe('c11');
    c.select('c1'); // a pool card, not in the pack
    expect(c.selected).toBe('c11');
    c.select(null);
    expect(c.selected).toBeNull();
  });

  it('pick sends only (round, pick_no, id) and adopts the server pack it gets back', async () => {
    const next = draftPack({ pick_no: 4, pack: [card('c20', 'Giant Growth'), card('c21', 'Shock')], pool: [...draftPack().pool, card('c11', 'Shivan Dragon', 'R')] });
    const { c, calls } = await drafting({ 'POST /api/events/e1/pick': { body: next } });
    c.select('c11');
    await c.pick();
    const post = calls.find((x) => x.method === 'POST')!;
    expect(post.path).toBe('/api/events/e1/pick');
    expect(post.body).toEqual({ round: 1, pick_no: 3, id: 'c11' });
    expect(post.auth).toBe('Bearer tok0');
    expect(c.pack?.pick_no).toBe(4);
    expect(c.pack?.pool.map((x) => x.id)).toContain('c11');
    expect(c.selected).toBeNull(); // the picked card left the pack
    expect(c.busy).toBe(false);
  });

  it('the double-click path picks the named card without a prior select', async () => {
    const { c, calls } = await drafting({ 'POST /api/events/e1/pick': { body: draftPack({ pick_no: 4, pack: [] }) } });
    await c.pick('c12');
    expect(calls.find((x) => x.method === 'POST')?.body).toEqual({ round: 1, pick_no: 3, id: 'c12' });
  });

  it('the last pick answers "waiting"; the client then refetches the state and sees BUILD', async () => {
    let stateGets = 0;
    const { c } = await drafting({
      'GET /api/events/e1': () => ({ body: eventState({ phase: stateGets++ === 0 ? 'DRAFT' : 'BUILD' }) }),
      'POST /api/events/e1/pick': { body: draftPack({ waiting: true, round: 0, pick_no: 0, pack: null as unknown as [] }) },
    });
    await c.pick('c10');
    expect(c.pack?.waiting).toBe(true);
    expect(c.pack?.pack).toEqual([]); // the server sends null for an empty pack
    expect(c.phase).toBe('BUILD');
  });

  it('a pick with nothing selected sends nothing', async () => {
    const { c, calls } = await drafting();
    await c.pick();
    expect(calls.some((x) => x.method === 'POST')).toBe(false);
  });

  it('a stale pick (409 stale_pick) is not retried: the client refetches and shows the pack the server holds now', async () => {
    let packGets = 0;
    const fresh = draftPack({ pick_no: 5, pack: [card('c30', 'Shock')] });
    const { c, calls } = await drafting({
      'GET /api/events/e1/pack': () => (packGets++ === 0 ? { body: draftPack() } : { body: fresh }),
      'POST /api/events/e1/pick': { status: 409, body: { code: 'stale_pick', message: "seat 0's pack is round 1 pick 5, not round 1 pick 3" } },
    });
    c.select('c11');
    await c.pick();
    expect(calls.filter((x) => x.method === 'POST')).toHaveLength(1);
    expect(c.pack?.pick_no).toBe(5);
    expect(c.selected).toBeNull();
    expect(c.notice).toMatch(/moved on/);
    expect(c.error).toBeNull();
  });

  it('not_your_turn and not_in_pack are handled the same way', async () => {
    for (const code of ['not_your_turn', 'not_in_pack']) {
      const { c } = await drafting({ 'POST /api/events/e1/pick': { status: code === 'not_in_pack' ? 400 : 409, body: { code, message: code } } });
      c.select('c10');
      await c.pick();
      expect(c.notice).toMatch(/moved on/);
      expect(c.error).toBeNull();
    }
  });

  it('any other failure is an error the player sees', async () => {
    const { c } = await drafting({ 'POST /api/events/e1/pick': { status: 503, body: { code: 'unavailable', message: 'event halted' } } });
    c.select('c10');
    await c.pick();
    expect(c.error).toBe('event halted');
    expect(c.selected).toBe('c10');
  });
});

describe('EventClient: frames', () => {
  it('applies only frames for its own event', async () => {
    const c = new EventClient('e1', seat);
    expect(c.handleFrame(frame('event_state', eventState({ event: 'e2' })))).toBe(false);
    expect(c.state).toBeNull();
    expect(c.handleFrame(frame('event_state', eventState()))).toBe(true);
    expect(c.state?.event).toBe('e1');
  });

  it('draft_pack replaces the pack, keeping the selection only while that card is on offer', () => {
    const c = new EventClient('e1', seat);
    c.handleFrame(frame('event_state', eventState()));
    c.handleFrame(frame('draft_pack', draftPack()));
    c.select('c10');
    c.handleFrame(frame('draft_pack', draftPack({ pick_no: 3 })));
    expect(c.selected).toBe('c10');
    c.handleFrame(frame('draft_pack', draftPack({ pick_no: 4, pack: [card('c40', 'Shock')] })));
    expect(c.selected).toBeNull();
    expect(c.pack?.pick_no).toBe(4);
  });

  it('ignores a draft_pack addressed to another seat', () => {
    const c = new EventClient('e1', seat);
    c.handleFrame(frame('event_state', eventState()));
    c.handleFrame(frame('draft_pack', draftPack({ seat: 2 })));
    expect(c.pack).toBeNull();
  });

  it('pod_progress feeds the queue counts', () => {
    const c = new EventClient('e1', seat);
    c.handleFrame(frame('pod_progress', { event: 'e1', round: 1, picks: [3, 3, 2, 2], queued: [1, 0, 2, 1] }));
    expect(c.progress?.queued).toEqual([1, 0, 2, 1]);
  });

  it('series_update patches one pairing row', () => {
    const c = new EventClient('e1', seat);
    const s = (over: Partial<EventSeries>): EventSeries => ({ round: 1, a: 0, b: 3, best_of: 1, state: 'pending', score: [0, 0], winner: null, ...over });
    c.handleFrame(frame('event_state', eventState({ phase: 'ROUND1', round: 1, series: [s({}), s({ a: 1, b: 2 })] })));
    c.handleFrame(frame('series_update', { event: 'e1', series: s({ a: 1, b: 2, state: 'ended', score: [1, 0], winner: 1, table: 'e1-r1-s12-g1-a1' }) }));
    expect(c.state?.series[1]).toMatchObject({ state: 'ended', winner: 1, table: 'e1-r1-s12-g1-a1' });
    expect(c.state?.series[0].state).toBe('pending');
  });
});

describe('patchSeries', () => {
  it('keeps the viewer-only join link a live update does not carry', () => {
    const live: EventSeries = { round: 1, a: 0, b: 3, best_of: 1, state: 'live', table: 't', score: [0, 0], winner: null, join: '/t/t?seat=0&token=x' };
    const out = patchSeries([live], { ...live, join: undefined, attempt: 1 });
    expect(out[0].join).toBe('/t/t?seat=0&token=x');
    const ended = patchSeries([live], { ...live, join: undefined, state: 'ended', winner: 0, score: [1, 0] });
    expect(ended[0].join).toBeUndefined();
  });
});

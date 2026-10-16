import { mount } from 'svelte';
import '../../app.css';
import Fixture from './TableEventOverlay.fixture.svelte';

// A series game's table page: the URL carries the GAME seat token and
// &event=, and the event page remembered the EVENT seat for e1. The event
// state answers a between-games row (?state=between) or a live one.
const mode = new URLSearchParams(location.search).get('state') ?? 'between';
history.replaceState(null, '', '/t/e1-r1-s03-g1-a1?seat=0&token=GAMEtok&event=e1');
localStorage.setItem('gorge.event.seat.e1', JSON.stringify({ seat: 0, token: 'EVENTtok0' }));
type W = { auths: string[] };
(window as unknown as W).auths = [];

const series = {
  round: 1, a: 0, b: 3, best_of: 3, table: 'e1-r1-s03-g1-a1', score: [0, 1], winner: null, games: [3], game: 2, chooser: 0,
  ...(mode === 'between' ? { state: 'between', waiting: [0, 3] } : { state: 'live', attempt: 1 }),
};
const seats = [0, 1, 2, 3].map((s) => ({ seat: s, kind: s === 0 || s === 3 ? 'human' : 'bot', name: ['Ada', 'Bot 2', 'Bot 3', 'Bea'][s], joined: true, picks: 42, ready: true }));
const state = { event: 'e1', set: 'BLB', phase: 'ROUND1', round: 1, best_of: 3, pick_timer: 'standard', seats, you: 0, series: [series], standings: [] };

window.fetch = async (input, init) => {
  const url = String(input);
  const auth = (init?.headers as Record<string, string> | undefined)?.Authorization;
  if (auth) (window as unknown as W).auths.push(auth);
  const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
  if (url.endsWith('/api/events/e1')) return json(state);
  if (url.endsWith('/api/events/e1/pool')) return json({ event: 'e1', seat: 0, pool: [], deck: null, rev: 1, ready: true, playing: { main: [], basics: [20, 0, 0, 0, 20] }, playing_rev: 1, suggestion: { main: [], basics: [0, 0, 0, 0, 0] } });
  if (url.endsWith('/api/events/e1/deck/check')) return json({ valid: true, total: 40, auto_lands: [0, 0, 0, 0, 0] });
  return new Response('{}', { status: 404 });
};

mount(Fixture, { target: document.getElementById('app')! });

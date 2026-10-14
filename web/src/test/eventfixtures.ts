/**
 * Test fixtures for the draft-event screens: a server-shaped EventState,
 * DraftPack and EventPool, and a fetch double that answers /api/events
 * routes from a handler table and records every request. Test-only; no
 * production module imports it.
 */
import { vi } from 'vitest';
import type { DraftCard, DraftPack, EventPool, EventState } from '../protocol';
import type { CardFacts } from '../lib/draftcards';

export const card = (id: string, name: string, rarity = 'C'): DraftCard => ({ id, name, set: 'FDN', number: '1', rarity });

export function eventState(over: Partial<EventState> = {}): EventState {
  return {
    event: 'e1',
    set: 'FDN',
    phase: 'DRAFT',
    round: 0,
    best_of: 1,
    you: 0,
    seats: [
      { seat: 0, kind: 'human', name: 'Ada', joined: true, picks: 2, ready: false },
      { seat: 1, kind: 'bot', name: 'Bot 2 (lethal-pressure)', policy: 'lethal-pressure', drafter: 'rating-colour', joined: true, picks: 2, ready: false },
      { seat: 2, kind: 'bot', name: 'Bot 3 (bot)', policy: 'bot', drafter: 'rating-colour', joined: true, picks: 2, ready: false },
      { seat: 3, kind: 'bot', name: 'Bot 4 (bot)', policy: 'bot', drafter: 'rating-colour', joined: true, picks: 2, ready: false },
    ],
    series: [],
    standings: [],
    ...over,
  };
}

export function draftPack(over: Partial<DraftPack> = {}): DraftPack {
  return {
    event: 'e1',
    seat: 0,
    waiting: false,
    round: 1,
    pick_no: 3,
    pack: [card('c10', 'Llanowar Elves'), card('c11', 'Shivan Dragon', 'R'), card('c12', 'Giant Growth')],
    pool: [card('c1', 'Serra Angel', 'U'), card('c2', 'Lightning Bolt')],
    ...over,
  };
}

export function eventPool(over: Partial<EventPool> = {}): EventPool {
  return {
    event: 'e1',
    seat: 0,
    pool: [card('c1', 'Serra Angel'), card('c2', 'Lightning Bolt'), card('c3', 'Llanowar Elves'), card('c4', 'Giant Growth')],
    deck: null,
    rev: 0,
    ready: false,
    suggestion: { main: ['c2', 'c3'], basics: [0, 0, 0, 8, 9] },
    ...over,
  };
}

export const FACTS: Record<string, CardFacts> = {
  'Serra Angel': { mv: 5, colours: ['W'], land: false, creature: true, cost: '3 W W', types: 'Creature — Angel' },
  'Lightning Bolt': { mv: 1, colours: ['R'], land: false, creature: false, cost: 'R', types: 'Instant' },
  'Llanowar Elves': { mv: 1, colours: ['G'], land: false, creature: true, cost: 'G', types: 'Creature — Elf Druid' },
  'Giant Growth': { mv: 1, colours: ['G'], land: false, creature: false, cost: 'G', types: 'Instant' },
  'Shivan Dragon': { mv: 6, colours: ['R'], land: false, creature: true, cost: '4 R R', types: 'Creature — Dragon' },
  'Evolving Wilds': { mv: 0, colours: [], land: true, creature: false, cost: '', types: 'Land' },
};

export interface Recorded {
  method: string;
  path: string;
  auth: string | null;
  body: unknown;
}

type Reply = { status?: number; body?: unknown };
type Handler = (req: Recorded) => Reply | Promise<Reply>;

/**
 * fakeFetch installs a global fetch that routes "METHOD /path" (query
 * stripped) to a handler. An unrouted request answers 500 so a test sees it.
 */
export function fakeFetch(routes: Record<string, Handler | Reply>) {
  const calls: Recorded[] = [];
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0];
    const method = init?.method ?? 'GET';
    const headers = (init?.headers ?? {}) as Record<string, string>;
    const rec: Recorded = { method, path, auth: headers.Authorization ?? null, body: init?.body ? JSON.parse(String(init.body)) : undefined };
    calls.push(rec);
    const h = routes[`${method} ${path}`];
    const reply = typeof h === 'function' ? await h(rec) : (h ?? { status: 500, body: { code: 'internal', message: `unrouted ${method} ${path}` } });
    const status = reply.status ?? 200;
    return new Response(status === 204 ? null : JSON.stringify(reply.body ?? {}), { status, headers: { 'Content-Type': 'application/json' } });
  });
  vi.stubGlobal('fetch', fn);
  return { calls, fn };
}

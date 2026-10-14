import { describe, expect, it, vi } from 'vitest';
import type { TableInfo } from '../protocol';
import type { TableState } from './tables.svelte';

// tables.svelte.ts wires its constructor to session.stream.onFrame, and the
// real session opens a browser EventSource — stub session so this stays a
// pure, hermetic test of apply() with hand-built frames.
const { fakeSession } = vi.hoisted(() => {
  const fakeSession = { stream: { onFrame: (): (() => void) => () => {} } };
  return { fakeSession };
});
vi.mock('./session.svelte', () => ({ session: fakeSession }));

const fetchTablesMock = vi.fn();
vi.mock('./api', () => ({ fetchTables: fetchTablesMock }));

const { tables } = await import('./tables.svelte');

// TableInfo.series is E3-fx2's field; the local alias lets a test build one
// before the pinned protocol.ts carries it.
type TableInfoWithSeries = TableInfo & { series?: boolean };
const info = (id: string, overrides: Partial<TableInfoWithSeries> = {}): TableInfoWithSeries =>
  ({ id, name: id, seats: 2, spectator: '', state: 'idle', match: 0, perpetual: false, format: 'constructed', bot_policy: 'bot', mulligans: 6, auto_mana: false, ...overrides });

function find(id: string): TableState {
  const t = tables.list.find((x) => x.info.id === id);
  if (!t) throw new Error(`no table ${id} in the store`);
  return t;
}

describe('tables: series table state (E3-fx2 client)', () => {
  it('a table_state frame sets the state, mapping between_games to cooldown', () => {
    tables.apply({ v: 1, t: 'hello', seq: 0, body: { session: 's1', tables: [info('s1', { series: true })] } });
    expect(find('s1').info.state).toBe('idle'); // precondition: a fresh table is idle

    tables.apply({ v: 1, t: 'table_state', seq: 1, table: 's1', body: { state: 'between_games' } });
    expect(find('s1').info.state).toBe('cooldown');

    tables.apply({ v: 1, t: 'table_state', seq: 2, table: 's1', body: { state: 'live' } });
    expect(find('s1').info.state).toBe('live');

    tables.apply({ v: 1, t: 'table_state', seq: 3, table: 's1', body: { state: 'idle' } });
    expect(find('s1').info.state).toBe('idle');
  });

  it("a series table's match_end is between games, not idle; a plain table's is idle", () => {
    tables.apply({ v: 1, t: 'hello', seq: 0, body: { session: 's1', tables: [info('ser', { series: true }), info('plain')] } });
    expect((find('ser').info as TableInfoWithSeries).series).toBe(true); // precondition: the store kept the series flag
    expect((find('plain').info as TableInfoWithSeries).series).toBeFalsy();

    tables.apply({ v: 1, t: 'match_end', seq: 1, table: 'ser', body: { result: 'win', winner: 0, head: 'h' } });
    expect(find('ser').info.state).toBe('cooldown');

    tables.apply({ v: 1, t: 'match_end', seq: 2, table: 'plain', body: { result: 'win', winner: 0, head: 'h' } });
    expect(find('plain').info.state).toBe('idle');
  });

  it('a table_state frame for an unknown table is ignored', () => {
    tables.apply({ v: 1, t: 'hello', seq: 0, body: { session: 's1', tables: [info('known')] } });
    tables.apply({ v: 1, t: 'table_state', seq: 1, table: 'ghost', body: { state: 'live' } });
    expect(find('known').info.state).toBe('idle');
  });
});

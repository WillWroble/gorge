import { describe, expect, it } from 'vitest';
import { href, parseRoute } from './router';

describe('router', () => {
  it('parses the three routes and rejects the rest', () => {
    expect(parseRoute('/')).toEqual({ kind: 'overview' });
    expect(parseRoute('/t/t1')).toEqual({ kind: 'table', table: 't1' });
    expect(parseRoute('/t/t1/m/7')).toEqual({ kind: 'match', table: 't1', match: 7 });
    expect(parseRoute('/t/t1/m/x')).toEqual({ kind: 'notfound' });
    expect(parseRoute('/nope')).toEqual({ kind: 'notfound' });
  });
  it('round-trips through href', () => {
    for (const p of ['/', '/t/t1', '/t/t1/m/7', '/events/new', '/e/e3']) expect(href(parseRoute(p))).toBe(p);
  });
  it('parses the draft-event routes', () => {
    expect(parseRoute('/events/new')).toEqual({ kind: 'newEvent' });
    expect(parseRoute('/e/e12')).toEqual({ kind: 'event', event: 'e12' });
    expect(parseRoute('/e/e12/')).toEqual({ kind: 'event', event: 'e12' });
    expect(parseRoute('/e/a%20b')).toEqual({ kind: 'event', event: 'a b' });
    expect(parseRoute('/e/')).toEqual({ kind: 'notfound' });
    expect(parseRoute('/e/e1/pack')).toEqual({ kind: 'notfound' });
    expect(parseRoute('/events')).toEqual({ kind: 'notfound' });
    expect(href({ kind: 'event', event: 'a b' })).toBe('/e/a%20b');
  });
});

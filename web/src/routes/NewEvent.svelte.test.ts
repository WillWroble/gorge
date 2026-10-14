import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import NewEvent from './NewEvent.svelte';
import DraftEntry from '../components/DraftEntry.svelte';

// SSR, the repo's component-test convention: onMount never runs, so nothing
// is fetched; sets0/personas0 stand in for the two discovery replies. The
// request the button sends is pinned in lib/eventmisc.test.ts (createRequest).
const SETS = [
  { code: 'BLB', name: 'Bloomburrow', pack_size: 14, ratings: 'heuristic' },
  { code: 'FDN', name: 'Foundations', pack_size: 14, ratings: '17lands' },
];
describe('/events/new', () => {
  it('renders the 4-seat pod: you plus three bot seats with a policy and persona select each, Bo1, and the server\'s sets', () => {
    const { body } = render(NewEvent, { props: { sets0: SETS, personas0: ['best23', 'rating-colour'] } });
    expect(body).toContain('Events / New');
    expect(body).toContain('<b>You</b>');
    for (const n of [2, 3, 4]) expect(body).toContain(`data-testid="seat-${n}-policy"`);
    expect(body.match(/>Plays: server default<\/option>/g)).toHaveLength(3);
    expect(body).toContain('Best of 1');
    expect(body).toMatch(/<button[^>]*disabled[^>]*>Best of 3 \(later\)/);
    expect(body).toContain('value="FDN"');
    expect(body).toContain('value="BLB"');
    expect(body).toContain('Bloomburrow');
    expect(body).toContain('14-card packs · ratings: card heuristic');
    expect(body).toContain('14-card packs · ratings: 17Lands');
    for (const n of [2, 3, 4]) expect(body).toContain(`data-testid="seat-${n}-drafter"`);
    expect(body.match(/>Drafter: best23<\/option>/g)).toHaveLength(3);
    expect(body.match(/>Drafter: server default \(rating-colour\)<\/option>/g)).toHaveLength(3);
    // BLB (heuristic) is the first set, so it is chosen and no 17Lands credit shows.
    expect(body).not.toContain('data-testid="attribution"');
    expect(body).toContain('Open the pod');
    expect(body).toContain('Only bots that cannot see your decklist are offered.');
  });

  it('a 17lands-rated set carries the credit; nothing is hard-coded before the sets load', () => {
    const rated = render(NewEvent, { props: { sets0: [SETS[1]], personas0: [] } }).body;
    expect(rated).toContain('data-testid="attribution"');
    const loading = render(NewEvent, {}).body;
    expect(loading).toContain('Loading the sets');
    expect(loading).not.toContain('value="FDN"');
    expect(loading).toMatch(/data-testid="open-pod"[^>]*disabled|disabled[^>]*data-testid="open-pod"/);
    const none = render(NewEvent, { props: { sets0: [] } }).body;
    expect(none).toContain('This server offers no draftable sets.');
  });
});

describe('Overview draft entry', () => {
  it('links to /events/new beside play-vs-bot', () => {
    const { body } = render(DraftEntry, {});
    expect(body).toContain('href="/events/new"');
    expect(body).toContain('>Draft<');
  });
});

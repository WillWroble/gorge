import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import NewEvent from './NewEvent.svelte';
import DraftEntry from '../components/DraftEntry.svelte';

// SSR, the repo's component-test convention: onMount never runs, so the bot
// policy list is still empty and nothing is fetched. The request the button
// sends is pinned in lib/eventmisc.test.ts (createRequest).
describe('/events/new', () => {
  it('renders the 4-seat pod: you plus three bot seats with a policy select each, Bo1, and the set list', () => {
    const { body } = render(NewEvent, {});
    expect(body).toContain('Events / New');
    expect(body).toContain('<b>You</b>');
    for (const n of [2, 3, 4]) expect(body).toContain(`data-testid="seat-${n}-policy"`);
    expect(body.match(/>Plays: server default<\/option>/g)).toHaveLength(3);
    expect(body).toContain('Best of 1');
    expect(body).toMatch(/<button[^>]*disabled[^>]*>Best of 3 \(later\)/);
    expect(body).toContain('value="FDN"');
    expect(body).toContain('value="BLB"');
    expect(body).toContain('Open the pod');
    expect(body).toContain('Only bots that cannot see your decklist are offered.');
  });
});

describe('Overview draft entry', () => {
  it('links to /events/new beside play-vs-bot', () => {
    const { body } = render(DraftEntry, {});
    expect(body).toContain('href="/events/new"');
    expect(body).toContain('>Draft<');
  });
});

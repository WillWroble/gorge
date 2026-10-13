import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import type { CardView, PlayerView, SeatInfo, View } from '../protocol';
import PassiveHandFan from './PassiveHandFan.svelte';
import BoardStage from './BoardStage.svelte';

// SSR (svelte/server), the repo's component-test pattern: what renders from a
// given view. The browser half — geometry, hover, the seated fan's CAST/PLAY
// — is SpectatorHands.geometry.test.ts.

const card = (id: number, name: string): CardView => ({
  id, name, types: 'Creature — Elf', mana_cost: '1 G', printing: { name }, token: '', tapped: false,
  power: 1, toughness: 1, damage: 0, attacking: false, controller: 0, owner: 0, summon_sick: false,
});
const player = (seat: number, hand: CardView[] | null): PlayerView => ({
  seat, name: `P${seat}`, life: 20, lost: false, spells_cast_this_turn: 0, life_gained_this_turn: 0, life_lost_this_turn: 0, tokens_created_this_turn: 0, library_size: 50, hand_size: 7, graveyard_size: 0,
  completed_dungeons: 0,
  hand: hand as CardView[], pool: null as unknown as Record<string, number>,
  battlefield: [], graveyard: [], exile: [], command: [], commanders: [], commander_casts: [],
});
const seats: SeatInfo[] = [{ name: 'P0', deck: 'd', colour: '#e5484d' }, { name: 'P1', deck: 'd', colour: '#30a46c' }, { name: 'P2', deck: 'd', colour: '#4a8fd4' }, { name: 'P3', deck: 'd', colour: '#d8a24a' }];
const view = (viewer: number, visibility: View['visibility'], players: PlayerView[]): View => ({
  viewer, visibility, turn: 2, round: 2, step: 'main1', phase: 'main1', active: 0, priority: 0,
  over: false, draw: false, winner: null, stack: [], pending: [], players,
});

describe('PassiveHandFan', () => {
  it('draws every card as a CardImage face with no control of any kind', () => {
    const { html } = render(PassiveHandFan, { props: { seat: 1, name: 'P1', cards: [card(11, 'Elf A'), card(12, 'Elf B')], faceH: 80, width: 400 } });
    expect(html).toContain('data-passive-hand="1"');
    expect(html.match(/data-hand-card=/g)).toHaveLength(2);
    expect(html.match(/class="card-image/g)).toHaveLength(2);
    expect(html).toContain('Elf A');
    expect(html).not.toContain('<button');
    expect(html).not.toContain('role="button"');
    expect(html).not.toContain('role="menu"');
    expect(html).not.toContain('data-payment-card');
    expect(html).not.toContain('data-play-land');
  });

  it('an empty exposed hand keeps its band and says so', () => {
    const { html } = render(PassiveHandFan, { props: { seat: 0, name: 'P0', cards: [], faceH: 60, width: 300 } });
    expect(html).toContain('data-passive-hand="0"');
    expect(html).toContain('No cards in hand');
    expect(html).not.toContain('data-hand-card');
  });
});

describe('Board — who gets a passive hand', () => {
  it('an omniscient spectator gets one per exposed seat, at two and four seats', () => {
    for (const n of [2, 4]) {
      const players = Array.from({ length: n }, (_, s) => player(s, [card(100 + s, `Hidden ${s}`)]));
      const { html } = render(BoardStage, { props: { view: view(255, 'omniscient', players), seats } });
      for (let s = 0; s < n; s++) expect(html, `${n} seats, seat ${s}`).toContain(`data-passive-hand="${s}"`);
      expect(html.match(/data-passive-hand=/g)).toHaveLength(n);
    }
  });

  it('a public spectator gets none and no hidden identity, whatever hand_size says', () => {
    const { html } = render(BoardStage, { props: { view: view(255, 'public', [player(0, null), player(1, null)]), seats } });
    expect(html).not.toContain('data-passive-hand');
    expect(html).not.toContain('Hidden');
  });

  it('only the exposed seats of a mixed projection get one', () => {
    const { html } = render(BoardStage, { props: { view: view(255, 'omniscient', [player(0, null), player(1, [card(101, 'Hidden 1')])]), seats } });
    expect(html.match(/data-passive-hand=/g)).toHaveLength(1);
    expect(html).toContain('data-passive-hand="1"');
  });

  it('a seated viewer gets none — not even for an opponent hand present in the payload', () => {
    const { html } = render(BoardStage, { props: { view: view(0, 'seat', [player(0, [card(100, 'Own')]), player(1, [card(101, 'Opp Hidden')])]), seats, seat: 0 } });
    expect(html).not.toContain('data-passive-hand');
    expect(html).not.toContain('Opp Hidden');
  });
});

import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import type { DraftCard } from '../protocol';
import { DWELL } from './carddetail.svelte';
import { DraftHover } from './drafthover.svelte';
import { cardView } from './draftcards';
import CardDetail from '../components/CardDetail.svelte';

function fakeTimer() {
  let clock = 0;
  const timers = new Map<number, { at: number; fn: () => void }>();
  let next = 1;
  return {
    env: {
      setTimeout: (fn: () => void, ms: number) => {
        const id = next++;
        timers.set(id, { at: clock + ms, fn });
        return id;
      },
      clearTimeout: (id: unknown) => void timers.delete(id as number),
    },
    tick(ms: number) {
      clock += ms;
      for (const [id, t] of [...timers]) {
        if (t.at <= clock) {
          timers.delete(id);
          t.fn();
        }
      }
    },
  };
}

const el = { getBoundingClientRect: () => ({ left: 10, top: 20, right: 110 }) } as unknown as Element;
const bear: DraftCard = { id: 'p1', name: 'Grizzly Bears', set: 'FDN', number: '1', rarity: 'C', mana_cost: '1 G', mana_value: 2, colors: 'G', type_line: 'Creature — Bear' };
const bolt: DraftCard = { id: 'p2', name: 'Lightning Bolt', set: 'FDN', number: '2', rarity: 'C', mana_cost: 'R', mana_value: 1, colors: 'R', type_line: 'Instant' };

describe('DraftHover', () => {
  it('opens after the dwell, anchored to the hovered card, and closes on leave', () => {
    const t = fakeTimer();
    const h = new DraftHover(t.env);
    h.enter(bear, el);
    expect(h.show).toBe(false);
    t.tick(DWELL);
    expect(h.show).toBe(true);
    expect(h.card?.id).toBe('p1');
    expect(h.anchor).toEqual({ left: 10, top: 20, right: 110 });
    h.leave();
    expect(h.show).toBe(false);
  });

  it('focus opens at once and Escape closes', () => {
    const h = new DraftHover(fakeTimer().env);
    h.focus(bolt, el);
    expect(h.show).toBe(true);
    h.keydown({ key: 'Escape' });
    expect(h.show).toBe(false);
  });

  it('closes when its card leaves the surface (picked under the pointer)', () => {
    const h = new DraftHover(fakeTimer().env);
    h.focus(bear, el);
    h.supervise([bear, bolt]);
    expect(h.show).toBe(true);
    h.supervise([bolt]);
    expect(h.show).toBe(false);
  });
});

describe('CardDetail over a draft card', () => {
  it('prints no undefined P/T for a creature the draft wire sends without one', () => {
    const { body } = render(CardDetail, {
      props: { card: cardView(bear), anchor: { left: 0, top: 0, right: 100 }, resolver: () => null },
    });
    expect(body).toContain('Grizzly Bears');
    expect(body).not.toContain('undefined');
    expect(body).not.toContain('#0');
  });

  it('prints the printed P/T from the catalog when it resolves', () => {
    const { body } = render(CardDetail, {
      props: {
        card: cardView(bear),
        anchor: { left: 0, top: 0, right: 100 },
        resolver: () => ({ name: 'Grizzly Bears', power: '2', toughness: '2' }) as never,
      },
    });
    expect(body).toContain('2/2');
    expect(body).not.toContain('undefined');
  });
});

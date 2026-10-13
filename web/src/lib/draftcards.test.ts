import { describe, expect, it } from 'vitest';
import { coloursOf, costTokens, factsFromOracle, manaValue, mvColumns, openedBy, passDirection, passesFilter, seatLabel, tint, UNKNOWN_FACTS } from './draftcards';
import { FACTS } from '../test/eventfixtures';

describe('cost parsing (display only)', () => {
  it('reads the catalog brace form and the Forge spaced form alike', () => {
    expect(costTokens('{2}{W}{W}')).toEqual(['2', 'W', 'W']);
    expect(costTokens('2 W W')).toEqual(['2', 'W', 'W']);
    expect(costTokens('{X}{R}')).toEqual(['X', 'R']);
    expect(costTokens('{1}{W/U}')).toEqual(['1', 'W/U']);
    expect(costTokens('{1}{G} // {3}{U}')).toEqual(['1', 'G']); // a split card sorts by its front half
    expect(costTokens('')).toEqual([]);
    expect(costTokens('no cost')).toEqual([]);
  });
  it('mana value and colours', () => {
    expect(manaValue(['2', 'W', 'W'])).toBe(4);
    expect(manaValue(['X', 'R'])).toBe(1);
    expect(manaValue(['2/W'])).toBe(2);
    expect(manaValue(['W/U', 'B/P'])).toBe(2);
    expect(coloursOf(['1', 'W/U', 'G'])).toEqual(['W', 'U', 'G']);
  });
  it('factsFromOracle: a miss is unknown, a land is a land', () => {
    expect(factsFromOracle(null)).toBe(UNKNOWN_FACTS);
    expect(factsFromOracle({ name: 'Evolving Wilds', mana_cost: '', type_line: 'Land' })).toMatchObject({ land: true, mv: 0 });
    expect(factsFromOracle({ name: 'Serra Angel', mana_cost: '{3}{W}{W}', type_line: 'Creature — Angel' })).toMatchObject({
      mv: 5, colours: ['W'], creature: true, cost: '3 W W',
    });
  });
});

describe('mvColumns', () => {
  const of = (n: string) => FACTS[n] ?? UNKNOWN_FACTS;
  it('buckets 0–1 … 6+ and Land, and adds "?" only while some card is unknown', () => {
    const cols = mvColumns(['Serra Angel', 'Lightning Bolt', 'Shivan Dragon', 'Evolving Wilds'], of);
    expect(cols.map((c) => c.label)).toEqual(['0–1', '2', '3', '4', '5', '6+', 'Land']);
    expect(cols[0].cards).toEqual(['Lightning Bolt']);
    expect(cols[4].cards).toEqual(['Serra Angel']);
    expect(cols[5].cards).toEqual(['Shivan Dragon']);
    expect(cols[6].cards).toEqual(['Evolving Wilds']);
    const withUnknown = mvColumns(['Serra Angel', 'Mystery Card'], of);
    expect(withUnknown.at(-1)).toMatchObject({ label: '?', cards: ['Mystery Card'] });
  });
});

describe('pod geometry', () => {
  it('packs 1 and 3 pass left, pack 2 right (spec §4.4)', () => {
    expect([1, 2, 3].map(passDirection)).toEqual(['left', 'right', 'left']);
  });
  it('names the seat that opened the pack at a pick', () => {
    expect(openedBy(0, 1, 1)).toBe(0); // your own pack
    expect(openedBy(0, 1, 2)).toBe(3); // left pass: it came from seat 3
    expect(openedBy(0, 1, 3)).toBe(2);
    expect(openedBy(0, 2, 2)).toBe(1); // right pass: it came from seat 1
    expect(openedBy(1, 3, 5)).toBe(1); // a full lap
  });
});

describe('filters and labels', () => {
  it('colour chips', () => {
    const gold = { ...UNKNOWN_FACTS, colours: ['W', 'U'] as const } as unknown as typeof UNKNOWN_FACTS;
    expect(passesFilter('all', UNKNOWN_FACTS)).toBe(true);
    expect(passesFilter('W', FACTS['Serra Angel'])).toBe(true);
    expect(passesFilter('R', FACTS['Serra Angel'])).toBe(false);
    expect(passesFilter('multi', gold)).toBe(true);
    expect(passesFilter('colourless', FACTS['Evolving Wilds'])).toBe(true);
    expect(tint(gold)).toBe('M');
    expect(tint(FACTS['Lightning Bolt'])).toBe('R');
    expect(tint(UNKNOWN_FACTS)).toBe('C');
  });
  it('seat labels', () => {
    expect(seatLabel(0, 'Ada', 0)).toBe('You');
    expect(seatLabel(2, 'Bot 3 (bot)', 0)).toBe('Seat 3 · Bot 3 (bot)');
    expect(seatLabel(1, '', null)).toBe('Seat 2');
  });
});

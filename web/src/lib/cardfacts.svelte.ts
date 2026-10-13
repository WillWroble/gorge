import { oracle, type OracleCard } from './oracle';
import { factsFromOracle, UNKNOWN_FACTS, type CardFacts } from './draftcards';

/**
 * CardFactsStore resolves card names to display facts (mana value, colours,
 * land/creature, cost) through the card-text catalog, once per name, and
 * holds them reactively so a column re-sorts the moment a lookup lands.
 * It is display-only: an unresolved name reads as UNKNOWN_FACTS.
 */
export class CardFactsStore {
  facts = $state<Record<string, CardFacts>>({});
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- private once-per-name bookkeeping, never rendered; facts is the reactive surface
  #asked = new Set<string>();
  #resolve: (name: string) => Promise<OracleCard | null>;

  constructor(resolve: (name: string) => Promise<OracleCard | null> = oracle.text, preset: Record<string, CardFacts> = {}) {
    this.#resolve = resolve;
    this.facts = { ...preset };
    for (const n of Object.keys(preset)) this.#asked.add(n);
  }

  /** of reads one name's facts (UNKNOWN until resolved). */
  of(name: string): CardFacts {
    return this.facts[name] ?? UNKNOWN_FACTS;
  }

  /** want starts a lookup for every name not asked about yet. */
  want(names: Iterable<string>): void {
    for (const name of names) {
      if (this.#asked.has(name)) continue;
      this.#asked.add(name);
      this.#resolve(name)
        .then((o) => {
          if (o) this.facts[name] = factsFromOracle(o);
          // A null is "no answer yet" as often as "no such card": the catalog
          // backs off for a minute after any failure. Let the next want() ask
          // again — a known miss is memoised by the catalog, so that costs no
          // request.
          else this.#asked.delete(name);
        })
        .catch(() => this.#asked.delete(name));
    }
  }
}

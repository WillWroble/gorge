import type { DeckCheck, DeckRequest, DraftCard, EventPool, EventState } from '../protocol';
import type { EventApi } from './eventapi';
import { eventApi } from './eventapi';
import { message } from './draftevent.svelte';
import type { ColourFilter } from './draftcards';

export type Basics = [number, number, number, number, number];

/** STEPPER_MAX mirrors limited/build.MaxBasicsPerType: a stepper stops there, and the server still validates whatever arrives. */
export const STEPPER_MAX = 40;

export type Zone = 'deck' | 'side';

/**
 * DeckBuilder is the BUILD screen's working copy of one seat's deck: which
 * pool card ids are maindeck, and how many of each basic.
 *
 * The client never decides legality. Every change is followed by a
 * POST deck/check, and the screen shows that DeckCheck — total, validity,
 * the server's error text — verbatim. "Auto" lands are the server's
 * auto_lands for the current main; "Reset" is the server's bot suggestion.
 * Ready saves the working copy (a new revision, which the server validates
 * again) and readies exactly that revision.
 */
export class DeckBuilder {
  pool = $state<DraftCard[]>([]);
  main = $state<string[]>([]);
  basics = $state<Basics>([0, 0, 0, 0, 0]);
  filter = $state<ColourFilter>('all');
  check = $state<DeckCheck | null>(null);
  /** rev is the latest saved revision; ready whether the seat has readied it. */
  rev = $state(0);
  ready = $state(false);
  loaded = $state(false);
  busy = $state(false);
  error = $state<string | null>(null);
  suggestion = $state<DeckRequest | null>(null);
  /** onstate receives the EventState a Ready answers with (the event page adopts it: the last Ready opens round 1). */
  onstate: ((st: EventState) => void) | null = null;
  #saved = $state<string>('');
  #checkSeq = 0;

  readonly id: string;
  readonly #token: string;
  readonly #api: EventApi;

  constructor(id: string, token: string, api: EventApi = eventApi) {
    this.id = id;
    this.#token = token;
    this.#api = api;
  }

  /** load reads the pool, then starts from the saved deck, else the bot suggestion. */
  async load(): Promise<void> {
    try {
      this.adopt(await this.#api.pool(this.id, this.#token));
    } catch (e) {
      this.error = message(e);
      return;
    }
    await this.recheck();
  }

  adopt(p: EventPool): void {
    this.pool = p.pool ?? [];
    this.rev = p.rev;
    this.ready = p.ready;
    this.suggestion = p.suggestion ? { main: p.suggestion.main ?? [], basics: p.suggestion.basics } : null;
    const start = p.deck ?? this.suggestion;
    this.setDeck(start ?? { main: [], basics: [0, 0, 0, 0, 0] });
    this.#saved = p.deck ? key(p.deck) : '';
    this.loaded = true;
  }

  get deck(): DeckRequest {
    return { main: [...this.main], basics: [...this.basics] as Basics };
  }

  /** dirty is true when the working copy differs from the latest saved revision (or nothing is saved). */
  get dirty(): boolean {
    return key(this.deck) !== this.#saved;
  }

  inDeck(id: string): boolean {
    return this.main.includes(id);
  }

  get deckCards(): DraftCard[] {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- a throwaway index per read; main is the reactive source
    const order = new Map(this.main.map((id, i) => [id, i]));
    return this.pool.filter((c) => order.has(c.id)).sort((a, b) => order.get(a.id)! - order.get(b.id)!);
  }

  get sideCards(): DraftCard[] {
    return this.pool.filter((c) => !this.main.includes(c.id));
  }

  /** move puts one pool card into a zone (toggles when no zone is named): the click/tap path and the drop path both land here. */
  move(id: string, to?: Zone): void {
    if (!this.pool.some((c) => c.id === id)) return;
    const there = this.main.includes(id);
    const want = to === undefined ? !there : to === 'deck';
    if (want === there) return;
    this.main = want ? [...this.main, id] : this.main.filter((x) => x !== id);
    void this.recheck();
  }

  /** step nudges one basic's count by delta, clamped to the stepper's range. */
  step(colour: number, delta: number): void {
    if (colour < 0 || colour > 4) return;
    const next = [...this.basics] as Basics;
    next[colour] = Math.min(STEPPER_MAX, Math.max(0, next[colour] + delta));
    if (next[colour] === this.basics[colour]) return;
    this.basics = next;
    void this.recheck();
  }

  setFilter(f: ColourFilter): void {
    this.filter = f;
  }

  /** autoLands replaces the basics with the server's auto-land split for the current main. */
  async autoLands(): Promise<void> {
    const c = await this.recheck();
    if (!c) return;
    this.basics = [...c.auto_lands] as Basics;
    await this.recheck();
  }

  /** resetToSuggestion restores the server's bot-built deck for this pool. */
  async resetToSuggestion(): Promise<void> {
    if (!this.suggestion) return;
    this.setDeck(this.suggestion);
    await this.recheck();
  }

  /** recheck asks the server about the working copy; an answer for an older copy is dropped. */
  async recheck(): Promise<DeckCheck | null> {
    const seq = ++this.#checkSeq;
    try {
      const c = await this.#api.check(this.id, this.#token, this.deck);
      if (seq !== this.#checkSeq) return null;
      this.check = c;
      this.error = null;
      return c;
    } catch (e) {
      if (seq === this.#checkSeq) this.error = message(e);
      return null;
    }
  }

  /** readyUp saves the working copy when it changed, then readies the saved revision. */
  async readyUp(): Promise<boolean> {
    if (this.busy) return false;
    this.busy = true;
    this.error = null;
    try {
      if (this.dirty || this.rev === 0) {
        const deck = this.deck;
        const saved = await this.#api.save(this.id, this.#token, deck);
        this.check = saved;
        if (saved.rev) this.rev = saved.rev;
        this.#saved = key(deck);
        this.ready = false;
      }
      const st = await this.#api.ready(this.id, this.#token, this.rev);
      this.ready = true;
      this.onstate?.(st);
      return true;
    } catch (e) {
      this.error = message(e);
      return false;
    } finally {
      this.busy = false;
    }
  }

  private setDeck(d: DeckRequest): void {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- a throwaway lookup inside one write
    const ids = new Set(this.pool.map((c) => c.id));
    this.main = (d.main ?? []).filter((id) => ids.has(id));
    this.basics = [...(d.basics ?? [0, 0, 0, 0, 0])] as Basics;
  }
}

function key(d: DeckRequest): string {
  return JSON.stringify({ m: [...(d.main ?? [])].sort(), b: d.basics });
}

/** basicsTotal sums a basics array. */
export const basicsTotal = (b: readonly number[]): number => b.reduce((a, n) => a + n, 0);

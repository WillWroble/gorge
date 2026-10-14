import type { DraftCard } from '../protocol';
import { HoverCard, type AnchorRect, type TimerEnv } from './carddetail.svelte';

/**
 * DraftHover drives the hover/focus CardDetail panel for the draft-event
 * screens (pack cards, pool and deck strips): the same panel, dwell and
 * keyboard contract a board CardTile has, over DraftCards. One controller
 * serves a whole surface: pointer enter arms the dwell for that card and
 * captures its rect as the panel's anchor; leave, blur or Escape close it.
 *
 * supervise is the lifecycle half (HoverCard.supervise's contract): a picked
 * card leaves the pack while the pointer is still over it, and a removed
 * element never fires pointerleave, so the surface re-feeds the cards it
 * still shows and a panel describing a card that is gone closes.
 */
export class DraftHover {
  readonly hover: HoverCard;
  card = $state<DraftCard | null>(null);
  anchor = $state<AnchorRect | null>(null);

  constructor(env: Partial<TimerEnv> = {}) {
    this.hover = new HoverCard(env);
  }

  /** show is true while the panel is open for a card with an anchor. */
  get show(): boolean {
    return this.hover.show && this.card !== null && this.anchor !== null;
  }

  /** enter arms the dwell for c, anchored to el. */
  enter(c: DraftCard, el: Element): void {
    if (this.card?.id !== c.id) this.hover.close();
    this.card = c;
    this.hover.arm(() => this.capture(el));
  }

  /** focus opens the panel for c at once (keyboard path). */
  focus(c: DraftCard, el: Element): void {
    this.card = c;
    this.hover.open(() => this.capture(el));
  }

  /** leave closes the panel and cancels an armed dwell (pointer leave, blur). */
  leave(): void {
    this.hover.close();
  }

  /** keydown: Escape closes the panel. */
  keydown(e: { key: string }): void {
    this.hover.keydown(e);
  }

  /** supervise closes the panel when its card is no longer among present. */
  supervise(present: readonly DraftCard[]): void {
    const c = this.card;
    if (c !== null && !present.some((p) => p.id === c.id)) this.hover.close();
  }

  private capture(el: Element): void {
    const r = el.getBoundingClientRect();
    this.anchor = { left: r.left, top: r.top, right: r.right };
  }
}

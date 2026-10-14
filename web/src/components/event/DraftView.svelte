<script lang="ts">
  import type { EventClient } from '../../lib/draftevent.svelte';
  import { cardView, factsOf, openedBy, passDirection, rarityLabel, seatLabel, COLOURS, tint } from '../../lib/draftcards';
  import CardImage from '../CardImage.svelte';
  import MvColumns from './MvColumns.svelte';
  import PodStatus from './PodStatus.svelte';
  import PickClock from './PickClock.svelte';
  import Attribution from './Attribution.svelte';
  import CardDetail from '../CardDetail.svelte';
  import { DraftHover } from '../../lib/drafthover.svelte';

  /**
   * The pick screen (mockup 3b). Click or tap a card to inspect it;
   * double-click it, press Enter on the selected card, or use "Take card" to
   * pick it. The pick is an intent — (round, pick_no, id) — and the screen
   * then shows whatever pack the server says the seat holds.
   */
  let { client, rated = true }: { client: EventClient; rated?: boolean } = $props();

  const pack = $derived(client.pack);
  const ev = $derived(client.state);
  const you = $derived(client.you);
  const cards = $derived(pack && !pack.waiting ? pack.pack : []);
  const pool = $derived(pack?.pool ?? []);
  const selected = $derived(cards.find((c) => c.id === client.selected) ?? null);
  const selectedFacts = $derived(selected ? factsOf(selected) : null);
  const round = $derived(pack && !pack.waiting ? pack.round : (client.progress?.round ?? 0));
  const dir = $derived(passDirection(round || 1));
  const opener = $derived(pack && !pack.waiting && you !== null ? openedBy(you, pack.round, pack.pick_no) : null);

  const colourCounts = $derived.by(() => {
    const out: Record<string, number> = { W: 0, U: 0, B: 0, R: 0, G: 0 };
    for (const c of pool) for (const col of factsOf(c).colours) out[col]++;
    return out;
  });

  // Hovering a pack card opens the same zoomed CardDetail panel the table
  // uses; a picked card leaves the pack under the pointer, so supervise
  // closes a panel whose card is gone.
  const detail = new DraftHover();
  $effect(() => detail.supervise(cards));

  function onkey(e: KeyboardEvent, id: string) {
    detail.keydown(e);
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault();
    if (client.selected === id) void client.pick(id);
    else client.select(id);
  }
</script>

<div class="ev-split draft">
  <section class="ev-main">
    {#if pack && !pack.waiting}
      <div class="packhead">
        <h2>Pack {pack.round} · Pick {pack.pick_no}</h2>
        <span class="data dim">
          {cards.length} {cards.length === 1 ? 'card' : 'cards'} ·
          {opener === null || opener === you ? 'your own pack' : `opened by ${seatLabel(opener, ev?.seats[opener]?.name, you)}`}
        </span>
        <span class="dir data">passing {dir} {dir === 'left' ? '←' : '→'}</span>
        {#if (pack.time_limit_ms ?? 0) > 0}
          <PickClock remainingMs={pack.remaining_ms ?? 0} arrivedAt={client.packAt} />
        {:else}
          <span class="data dim" data-testid="no-pick-timer">no pick timer</span>
        {/if}
        <span class="sp"></span>
        <button type="button" class="ev-btn primary" data-testid="take" disabled={!selected || client.busy} onclick={() => void client.pick()}>
          {client.busy ? 'Taking…' : selected ? `Take ${selected.name}` : 'Take card'}
        </button>
      </div>
      {#if client.notice}<p class="ev-notice" role="status">{client.notice}</p>{/if}
      {#if client.error}<p class="ev-error" role="alert">{client.error}</p>{/if}
      <div class="pack" role="listbox" aria-label="Pack" data-testid="pack">
        {#each cards as c (c.id)}
          <button
            type="button"
            role="option"
            class="pcard ev-tint-{tint(factsOf(c))}"
            class:sel={c.id === client.selected}
            aria-selected={c.id === client.selected}
            aria-label={`${c.name}, ${rarityLabel(c.rarity)}`}
            data-id={c.id}
            disabled={client.busy}
            onclick={() => client.select(c.id)}
            ondblclick={() => void client.pick(c.id)}
            onkeydown={(e) => onkey(e, c.id)}
            onpointerenter={(e) => detail.enter(c, e.currentTarget)}
            onpointerleave={() => detail.leave()}
            onfocus={(e) => detail.focus(c, e.currentTarget)}
            onblur={() => detail.leave()}
          >
            <CardImage card={cardView(c)} pt={false} />
            <span class="ev-rar {c.rarity}" title={rarityLabel(c.rarity)}></span>
          </button>
        {/each}
      </div>
      <p class="hint">Hover a card to zoom it · tap to select · double-click, or Enter twice, to take it.</p>
      {#if detail.show && detail.card && detail.anchor}
        <CardDetail card={cardView(detail.card)} anchor={detail.anchor} />
      {/if}
    {:else}
      <div class="waiting" role="status">
        <h2>Waiting for the next pack</h2>
        <p class="dim">The seat passing to you is still picking. Your pack arrives here the moment it does.</p>
        {#if client.error}<p class="ev-error" role="alert">{client.error}</p>{/if}
      </div>
    {/if}

    <div class="pool">
      <div class="row">
        <h3 class="ev-h5">Your picks · {pool.length}</h3>
        <span class="legend data">
          {#each COLOURS as col (col)}
            {#if colourCounts[col]}<span><span class="ev-pip {col}">{col}</span> {colourCounts[col]}</span>{/if}
          {/each}
        </span>
      </div>
      <MvColumns cards={pool} label="Your picks" />
    </div>
    {#if rated}<Attribution />{/if}
  </section>

  <aside class="ev-rail">
    <div class="block">
      <h3 class="ev-h5">Pod</h3>
      {#if ev}<PodStatus seats={ev.seats} {you} progress={client.progress} />{/if}
    </div>
    <div class="block">
      <h3 class="ev-h5">Inspect</h3>
      {#if selected}
        <div class="big"><CardImage card={cardView(selected)} size="large" pt={false} /></div>
        <p class="data dim center">
          {selectedFacts?.types || selected.name}
          {#if selectedFacts && selectedFacts.mv !== null}· MV {selectedFacts.mv}{/if}
          · {rarityLabel(selected.rarity)}
        </p>
      {:else}
        <p class="dim small">Tap a card to inspect it. Double-click picks it.</p>
      {/if}
    </div>
  </aside>
</div>

<style>
  .packhead {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--sp-2) var(--sp-3);
  }
  h2 {
    margin: 0;
    font: 600 1.5rem var(--font-serif);
    color: var(--ink);
  }
  .dim {
    color: var(--ink-dim);
  }
  .small {
    font-size: var(--t-12);
  }
  .center {
    text-align: center;
  }
  .dir {
    color: var(--gilt);
  }
  .sp {
    flex: 1;
  }
  /* The pack: card art at a readable size, as many per row as fit. */
  .pack {
    --card-w: 100%;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(7.25rem, 28vw), 1fr));
    gap: var(--sp-3) var(--sp-2);
  }
  .pcard {
    position: relative;
    display: block;
    padding: 0;
    border: 0;
    background: none;
    border-radius: var(--radius-card);
    cursor: pointer;
    transition: transform 0.12s;
    min-width: 0;
  }
  .pcard :global(.card-image) {
    width: 100%;
  }
  .pcard:hover:not([disabled]) {
    transform: translateY(-3px);
  }
  .pcard.sel {
    outline: 2px solid var(--gilt);
    outline-offset: 2px;
    transform: translateY(-4px);
  }
  .pcard .ev-rar {
    position: absolute;
    left: 0.35rem;
    bottom: 0.35rem;
    box-shadow: 0 0 0 2px rgb(0 0 0 / 0.55);
  }
  .hint {
    margin: 0;
    font-size: var(--t-11);
    color: var(--ink-faint);
  }
  .waiting {
    padding: var(--sp-6) 0;
  }
  .waiting p {
    margin: var(--sp-2) 0 0;
  }
  .pool {
    border-top: 1px solid var(--edge-felt);
    padding-top: var(--sp-2);
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--sp-2);
  }
  .legend {
    display: flex;
    gap: var(--sp-2);
    flex-wrap: wrap;
    color: var(--ink-dim);
  }
  .legend > span {
    display: inline-flex;
    align-items: center;
    gap: 3px;
  }
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
  }
  .big {
    --card-w-large: min(13rem, 100%);
    display: flex;
    justify-content: center;
  }
</style>

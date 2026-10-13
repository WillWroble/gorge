<script lang="ts">
  import type { DraftCard } from '../../protocol';
  import type { CardFactsStore } from '../../lib/cardfacts.svelte';
  import { mvColumns, tint } from '../../lib/draftcards';
  import ManaSymbols from '../ManaSymbols.svelte';

  /**
   * A pool or deck laid out in mana-value columns (0–1 … 6+, Land, and "?"
   * while a card's facts are unresolved). Each card is a one-line strip.
   * With `onactivate` the strips are buttons — click, tap or Enter moves the
   * card — and with `draggable` they also start an HTML5 drag carrying the
   * card's instance id; the drop target is the zone around this grid.
   */
  let {
    cards,
    facts,
    draggable = false,
    onactivate,
    label,
  }: {
    cards: DraftCard[];
    facts: CardFactsStore;
    draggable?: boolean;
    onactivate?: (id: string) => void;
    label?: string;
  } = $props();

  const cols = $derived(mvColumns(cards, (c) => facts.of(c.name)));
  let dragging = $state<string | null>(null);

  function dragstart(e: DragEvent, id: string) {
    if (!e.dataTransfer) return;
    e.dataTransfer.setData('text/plain', id);
    e.dataTransfer.effectAllowed = 'move';
    dragging = id;
  }
</script>

<div class="mvcols" aria-label={label}>
  {#each cols as col (col.key)}
    <div class="mvcol">
      <div class="h"><span>{col.label}</span><span>{col.cards.length || ''}</span></div>
      {#each col.cards as c (c.id)}
        {@const f = facts.of(c.name)}
        {#if onactivate}
          <button
            type="button"
            class="strip ev-tint-{tint(f)}"
            class:dragging={dragging === c.id}
            data-id={c.id}
            title={c.name}
            {draggable}
            ondragstart={(e) => dragstart(e, c.id)}
            ondragend={() => (dragging = null)}
            onclick={() => onactivate(c.id)}
          >
            <span class="nm">{c.name}</span>
            {#if f.cost}<ManaSymbols cost={f.cost} />{/if}
          </button>
        {:else}
          <div class="strip ev-tint-{tint(f)}" data-id={c.id} title={c.name}>
            <span class="nm">{c.name}</span>
            {#if f.cost}<ManaSymbols cost={f.cost} />{/if}
          </div>
        {/if}
      {/each}
    </div>
  {/each}
</div>

<style>
  /* Seven columns on a wide screen; they wrap rather than shrink to nothing
     on a phone, so the page never scrolls sideways. */
  .mvcols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(6.5rem, 1fr));
    gap: 0.4rem;
  }
  .mvcol {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }
  .h {
    font: 500 var(--t-10) var(--font-data);
    color: var(--ink-faint);
    letter-spacing: 0.06em;
    padding-bottom: 3px;
    border-bottom: 1px solid var(--edge-felt);
    display: flex;
    justify-content: space-between;
  }
  .strip {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 4px;
    width: 100%;
    padding: 0.3rem 0.4rem;
    border: 0;
    border-left: 3px solid var(--cc, var(--mana-c));
    border-radius: 3px;
    background: color-mix(in srgb, var(--cc, var(--mana-c)) 20%, var(--felt-sunk));
    font: 500 var(--t-11) / 1.2 var(--font-ui);
    color: var(--ink);
    text-align: left;
    min-width: 0;
    /* a comfortable tap target on a phone */
    min-height: 1.75rem;
  }
  button.strip {
    cursor: pointer;
  }
  button.strip[draggable='true'] {
    cursor: grab;
  }
  button.strip:hover {
    background: color-mix(in srgb, var(--cc, var(--mana-c)) 32%, var(--felt-sunk));
  }
  .strip.dragging {
    opacity: 0.4;
  }
  .nm {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>

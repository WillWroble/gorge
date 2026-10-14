<script lang="ts">
  import type { EventSeat, PodProgress } from '../../protocol';
  import { seatLabel } from '../../lib/draftcards';

  /**
   * The pod (mockup 3b rail): one box per seat with its public pick count
   * and how many packs wait in its queue. Picks come from the live
   * pod_progress frame (the server sends one as the stream subscribes), else
   * from EventState; queue counts exist only in pod_progress, so a seat
   * whose count has not arrived simply shows its picks.
   */
  let { seats, you, progress = null }: { seats: EventSeat[]; you: number | null; progress?: PodProgress | null } = $props();

  const MAX_DOTS = 3;
</script>

<div class="pod" data-testid="pod">
  {#each seats as s (s.seat)}
    {@const picks = progress?.picks?.[s.seat] ?? s.picks}
    {@const queued = progress?.queued?.[s.seat]}
    <div class="seat" class:me={s.seat === you}>
      <b>{seatLabel(s.seat, s.name, you)}</b>
      <span class="data dim">{s.kind === 'bot' ? `bot · ${s.policy ?? ''}` : s.joined ? 'human' : 'human · not joined'}</span>
      <span class="data dim">picks {picks}{queued !== undefined ? ` · queued ${queued}` : ''}</span>
      {#if queued !== undefined}
        <span class="qdots" aria-label={`${queued} packs queued`}>
          {#each Array(MAX_DOTS) as _, j (j)}<i class:on={j < queued}></i>{/each}
        </span>
      {/if}
    </div>
  {/each}
</div>

<style>
  .pod {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(7rem, 1fr));
    gap: var(--sp-2);
  }
  .seat {
    border: 1px solid var(--edge-inst);
    border-radius: var(--radius);
    padding: 0.4rem 0.5rem;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--t-12);
    background: var(--instrument-raised);
    min-width: 0;
  }
  .seat b {
    font: 600 var(--t-12) var(--font-ui);
    color: var(--ink);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .seat.me {
    border-color: var(--gilt);
  }
  .dim {
    color: var(--ink-dim);
    font-size: var(--t-11);
    overflow-wrap: anywhere;
  }
  .qdots {
    display: flex;
    gap: 3px;
  }
  .qdots i {
    width: 0.55rem;
    height: 0.75rem;
    border-radius: 2px;
    background: var(--edge-inst);
  }
  .qdots i.on {
    background: var(--ink-dim);
  }
</style>

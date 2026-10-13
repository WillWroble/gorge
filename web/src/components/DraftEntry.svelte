<script lang="ts">
  import { onMount } from 'svelte';
  import type { EventState } from '../protocol';
  import { ApiError } from '../lib/api';
  import { listEvents } from '../lib/eventapi';

  /**
   * The lobby's draft-event entry point, beside "Play 1v1 vs a bot": a
   * button to /events/new and the events the server is running, each a
   * spectator link to its /e/{id} page. A server started without -events
   * answers GET /api/events with 404, and then this card hides itself.
   */
  let events = $state<EventState[]>([]);
  let enabled = $state(true);

  onMount(() => {
    listEvents()
      .then((l) => (events = l ?? []))
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 404) enabled = false;
      });
  });

  const running = $derived(events.filter((e) => e.phase !== 'COMPLETE'));
  const phaseLabel = (p: string) => (p === 'COMPLETE' ? 'results' : p.startsWith('ROUND') ? `round ${p.slice(5)}` : p.toLowerCase());
</script>

{#if enabled}
  <div class="draftentry" data-testid="draft-entry">
    <div class="text">
      <h2>Draft a 4-player pod</h2>
      <p class="hint">Open three packs with three bots, build a 40-card deck, then play a best-of-one round robin.</p>
    </div>
    <a class="draft" href="/events/new" data-testid="draft-button">Draft</a>
    {#if running.length > 0}
      <ul class="events">
        {#each running as e (e.event)}
          <li><a href={`/e/${encodeURIComponent(e.event)}`}>{e.set} · {e.event}</a> <span class="phase">{phaseLabel(e.phase)}</span></li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .draftentry {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-2) var(--sp-4);
    padding: var(--sp-3) var(--sp-4);
    border: 1px solid var(--edge-inst);
    border-radius: var(--radius);
    background: var(--instrument);
    color: var(--ink-inst);
  }
  .text {
    flex: 1 1 16rem;
    min-width: 0;
  }
  h2 {
    margin: 0;
    font-size: var(--t-14);
    font-weight: 600;
    color: var(--ink);
  }
  .hint {
    margin: 0;
    font-size: var(--t-12);
    color: var(--ink-dim);
  }
  .draft {
    background: var(--gilt);
    color: var(--felt-sunk);
    border-radius: var(--radius);
    padding: 0.4rem 1.1rem;
    font-weight: 600;
    font-size: var(--t-14);
    text-decoration: none;
  }
  .events {
    flex-basis: 100%;
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-1) var(--sp-4);
    font-family: var(--font-data);
    font-size: var(--t-11);
  }
  .events a {
    color: var(--ink-inst);
  }
  .phase {
    color: var(--ink-faint);
  }
</style>

<script lang="ts">
  /**
   * The lobby's draft-event entry point, beside "Play 1v1 vs a bot": a
   * button to /events/new. Like PlayVsBot it never probes the server from
   * the lobby — a server started without -events answers the create call
   * with 404 and the create page renders that error — so the lobby makes no
   * request that can fail on a server that does not host events (the smoke
   * gate counts any 4xx as a browser failure).
   */
</script>

<div class="draftentry" data-testid="draft-entry">
  <div class="text">
    <h2>Draft a 4-player pod</h2>
    <p class="hint">Open three packs with three bots, build a 40-card deck, then play a best-of-one round robin.</p>
  </div>
  <a class="draft" href="/events/new" data-testid="draft-button">Draft</a>
</div>

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
</style>

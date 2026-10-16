<script lang="ts">
  import { pickSecondsLeft, WARN_SECONDS } from '../../lib/pickclock';

  /**
   * The pick countdown (mockup 3b). The server armed the deadline and sent
   * remainingMs with the pack; this only ticks it down once a second from
   * arrivedAt, the moment the pack arrived. At zero it says
   * "auto-picking…": the server picks, and its next draft_pack (a new
   * arrivedAt) restarts the count.
   */
  let { remainingMs, arrivedAt }: { remainingMs: number; arrivedAt: number } = $props();

  let now = $state(Date.now());

  $effect(() => {
    // Re-armed for every pack: read arrivedAt so a new pack restarts the tick.
    void arrivedAt;
    now = Date.now();
    const id = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(id);
  });

  const secs = $derived(pickSecondsLeft(remainingMs, arrivedAt, now));
</script>

<span class="clock data" class:warn={secs < WARN_SECONDS} class:out={secs === 0} role="timer" aria-live="off" data-testid="pick-clock">
  {secs === 0 ? 'auto-picking…' : `${secs}s`}
</span>

<style>
  .clock {
    display: inline-block;
    min-width: 3.5rem;
    text-align: center;
    padding: 0.1rem 0.5rem;
    border-radius: var(--radius);
    border: 1px solid var(--edge-inst);
    background: var(--instrument-raised);
    color: var(--ink);
    font-size: var(--t-14);
  }
  .clock.warn {
    border-color: var(--ember);
    color: var(--ink);
    background: color-mix(in srgb, var(--ember) 18%, var(--instrument-raised));
  }
  .clock.out {
    color: var(--ink-dim);
  }
</style>

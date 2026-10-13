<script lang="ts">
  import type { EventSeries, EventState } from '../../protocol';
  import { withBase } from '../../lib/basepath';
  import { seatLabel } from '../../lib/draftcards';
  import { standingRows } from '../../lib/standings';
  import { countNames, pickHistory, revealDecks } from '../../lib/eventlog';
  import Standings from './Standings.svelte';

  /**
   * The event hub (mockup 3d) for ROUND1..3, and the results page for
   * COMPLETE. Pairings, scores and standings are the server's; the viewer's
   * own live game carries the server's seat-tokenised join path, every
   * other game a spectate link to its table.
   */
  let { ev, log = null }: { ev: EventState; log?: unknown[] | null } = $props();

  const you = $derived(ev.you ?? null);
  const complete = $derived(ev.phase === 'COMPLETE');
  const rounds = $derived.by(() => {
    const out: { round: number; series: EventSeries[] }[] = [];
    for (const s of ev.series ?? []) {
      let r = out.find((x) => x.round === s.round);
      if (!r) out.push((r = { round: s.round, series: [] }));
      r.series.push(s);
    }
    // the current round first, then the finished ones newest first, then the rest
    return out.sort((a, b) => rank(a.round) - rank(b.round));
  });
  const rows = $derived(standingRows(ev.standings ?? [], ev.seats ?? [], ev.series ?? [], you));
  const decks = $derived(complete && log ? revealDecks(log) : []);

  function rank(r: number): number {
    if (r === ev.round && !complete) return 0;
    if (r < ev.round || complete) return 10 - r;
    return 20 + r;
  }

  function label(seat: number): string {
    return seatLabel(seat, ev.seats?.[seat]?.name, you);
  }

  function roundStatus(series: EventSeries[]): string {
    if (series.every((s) => s.state === 'ended')) return 'complete';
    if (series.some((s) => s.state === 'live')) return 'playing';
    return 'upcoming';
  }

  function score(s: EventSeries): string {
    if (s.state === 'pending') return 'vs';
    return `${s.score?.[0] ?? 0} – ${s.score?.[1] ?? 0}`;
  }

  function outcome(s: EventSeries, seat: number): 'w' | 'l' | 'd' | '' {
    if (s.state !== 'ended') return '';
    if (s.winner === null || s.winner === undefined) return 'd';
    return s.winner === seat ? 'w' : 'l';
  }
</script>

<div class="ev-split hub">
  <section class="ev-main">
    {#if complete}
      <h2 class="title">Event complete</h2>
    {/if}
    {#each rounds as r (r.round)}
      <h3 class="ev-h5">Round {r.round} · {roundStatus(r.series)}</h3>
      {#each r.series as s (`${s.round}-${s.a}-${s.b}`)}
        {@const mine = you !== null && (s.a === you || s.b === you)}
        <div class="pair" class:mine data-testid="pairing">
          <div class="who"><b class={outcome(s, s.a)}>{label(s.a)}</b></div>
          <div class="mid">
            <div class="score data">{score(s)}</div>
            <div class="state data">{s.state === 'live' ? `live${s.attempt && s.attempt > 1 ? ` · attempt ${s.attempt}` : ''}` : s.state === 'ended' ? (s.winner === null ? 'draw' : 'final') : 'pending'}</div>
          </div>
          <div class="who r"><b class={outcome(s, s.b)}>{label(s.b)}</b></div>
          <div class="act">
            {#if s.join}
              <a class="ev-btn primary" href={withBase(s.join)} data-testid="join-game">Play your game</a>
            {:else if s.table && s.state === 'live'}
              <a class="ev-btn" href={withBase(`/t/${encodeURIComponent(s.table)}`)}>Watch</a>
            {:else if s.table && s.state === 'ended'}
              <a class="ev-btn sm" href={withBase(`/t/${encodeURIComponent(s.table)}`)}>Replay</a>
            {:else if mine && s.state === 'live'}
              <span class="data dim">starting…</span>
            {/if}
          </div>
        </div>
      {/each}
    {/each}

    {#if complete}
      <h3 class="ev-h5">Decks</h3>
      {#if decks.length === 0}
        <p class="dim">The revealed decks are loading.</p>
      {/if}
      <div class="decks">
        {#each decks as d (d.seat)}
          <details class="deck" data-testid="revealed-deck">
            <summary><b>{label(d.seat)}</b> <span class="data dim">{d.total} cards</span></summary>
            <ul>
              {#each countNames(d.cards) as c (c.name)}<li><span class="data">{c.count}</span> {c.name}</li>{/each}
              {#each d.basics as b (b.name)}<li><span class="data">{b.count}</span> {b.name}</li>{/each}
            </ul>
            {#if log}
              {@const picks = pickHistory(log, d.seat)}
              {#if picks.length}
                <details>
                  <summary class="dim">Picks</summary>
                  <ol class="picks">
                    {#each picks as p, i (i)}<li><span class="data dim">P{p.round}p{p.pickNo}</span> {p.name}{#if p.auto} <span class="dim">(auto)</span>{/if}</li>{/each}
                  </ol>
                </details>
              {/if}
            {/if}
          </details>
        {/each}
      </div>
    {/if}
  </section>

  <aside class="ev-rail">
    <Standings {rows} final={complete} />
  </aside>
</div>

<style>
  /* the standings table needs a wider rail than the draft's pod boxes */
  .hub {
    grid-template-columns: minmax(0, 1fr) 22rem;
  }
  @media (max-width: 760px) {
    .hub {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .title {
    margin: 0;
    font: 600 1.5rem var(--font-serif);
  }
  .dim {
    color: var(--ink-dim);
  }
  .pair {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
    gap: var(--sp-3);
    align-items: center;
    padding: var(--sp-2) var(--sp-3);
    background: var(--felt-raised);
    border: 1px solid var(--edge-felt);
    border-radius: var(--radius);
  }
  .pair.mine {
    border-color: var(--gilt);
  }
  .who {
    min-width: 0;
  }
  .who b {
    font: 600 1.1rem var(--font-serif);
    color: var(--ink);
    overflow-wrap: anywhere;
  }
  .who b.w {
    color: var(--verdigris);
  }
  .who b.l {
    color: var(--ink-dim);
  }
  .who.r {
    text-align: right;
  }
  .mid {
    text-align: center;
  }
  .score {
    font-size: 1.1rem;
    color: var(--ink);
  }
  .state {
    font-size: var(--t-10);
    color: var(--ink-faint);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .act {
    display: flex;
    justify-content: flex-end;
  }
  /* On a phone the action drops under the pairing instead of squeezing it. */
  @media (max-width: 520px) {
    .pair {
      grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    }
    .act {
      grid-column: 1 / -1;
      justify-content: center;
    }
  }
  .decks {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(16rem, 100%), 1fr));
    gap: var(--sp-2);
  }
  .deck {
    background: var(--felt-raised);
    border: 1px solid var(--edge-felt);
    border-radius: var(--radius);
    padding: var(--sp-2) var(--sp-3);
    font-size: var(--t-12);
  }
  .deck summary {
    cursor: pointer;
  }
  .deck ul,
  .deck ol {
    margin: var(--sp-2) 0 0;
    padding-left: 0;
    list-style: none;
  }
  .picks li {
    padding: 1px 0;
  }
</style>

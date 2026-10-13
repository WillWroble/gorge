<script lang="ts">
  import type { StandingRow } from '../../lib/standings';

  /**
   * The standings table: the server's rows in the server's order (MTR
   * Appendix C — match points, then OMW%, GW%, OGW%, with the 33% floor).
   * Nothing here re-ranks or recomputes a percentage.
   */
  let { rows, final = false }: { rows: StandingRow[]; final?: boolean } = $props();
</script>

<div class="tablebox">
  <table data-testid="standings">
    <caption class="ev-h5">{final ? 'Final standings' : 'Standings'}</caption>
    <thead>
      <tr>
        <th scope="col">#</th>
        <th scope="col">Seat</th>
        <th scope="col" title="Match points">Pts</th>
        <th scope="col" title="Match record (wins-losses)">W-L</th>
        <th scope="col" title="Opponents' match-win percentage">OMW%</th>
        <th scope="col" title="Game-win percentage">GW%</th>
        <th scope="col" title="Opponents' game-win percentage">OGW%</th>
      </tr>
    </thead>
    <tbody>
      {#each rows as r (r.seat)}
        <tr class:you={r.you} data-seat={r.seat}>
          <td class="data">{r.rank}</td>
          <td class="name">{#if r.you}<b>{r.label}</b>{:else}{r.label}{/if}</td>
          <td class="data">{r.points}</td>
          <td class="data">{r.record}</td>
          <td class="data">{r.omw}</td>
          <td class="data">{r.gw}</td>
          <td class="data">{r.ogw}</td>
        </tr>
      {:else}
        <tr><td colspan="7" class="empty">No results yet.</td></tr>
      {/each}
    </tbody>
  </table>
</div>
<p class="note">Order: match points, then OMW%, GW%, OGW% (MTR Appendix C, with the 33% floor).</p>

<style>
  .tablebox {
    overflow-x: auto;
    max-width: 100%;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--t-12);
  }
  caption {
    text-align: left;
    padding-bottom: var(--sp-2);
  }
  th {
    text-align: left;
    font: 500 var(--t-10) var(--font-data);
    letter-spacing: 0.06em;
    color: var(--ink-faint);
    border-bottom: 1px solid var(--edge-inst);
    padding: 0.3rem 0.4rem;
  }
  th,
  td.data {
    white-space: nowrap;
  }
  td {
    padding: 0.35rem 0.4rem;
    border-bottom: 1px solid color-mix(in srgb, var(--edge-inst) 50%, transparent);
    color: var(--ink-inst);
  }
  .name {
    overflow-wrap: anywhere;
  }
  tr.you td {
    color: var(--ink);
  }
  tr.you td:first-child {
    box-shadow: inset 2px 0 0 var(--gilt);
  }
  .empty {
    color: var(--ink-dim);
  }
  .note {
    margin: var(--sp-1) 0 0;
    font-size: var(--t-11);
    color: var(--ink-dim);
  }
</style>

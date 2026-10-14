<script lang="ts">
  import { onMount } from 'svelte';
  import '../components/event/event.css';
  import type { BotPolicyInfo, EventSetInfo } from '../protocol';
  import { fetchBotPolicies } from '../lib/api';
  import { createEvent, listEventPersonas, listEventSets } from '../lib/eventapi';
  import { navigateTo } from '../lib/router';
  import { message } from '../lib/draftevent.svelte';
  import { BOT_SEATS, DEFAULT_PERSONA, RATINGS_17LANDS, createRequest, eventPolicies, setNote } from '../lib/newevent';
  import Attribution from '../components/event/Attribution.svelte';

  /**
   * /events/new (mockup 3a): a 4-player pod, pick one, best of 1. Seat 1 is
   * you; seats 2–4 are bots. Everything offered is the server's: the sets
   * (GET /api/events/sets), the pickbot personas (GET /api/events/personas)
   * and the bot game policies (/api/bot-policies, which never lists a policy
   * that reads opponents' decklists). Creating the event sends you to your
   * own invite link.
   *
   * sets0 and personas0 seed the two catalogues for the SSR test harness,
   * where onMount never runs; production passes neither and fetches both.
   */
  let { sets0 = null, personas0 = null }: { sets0?: EventSetInfo[] | null; personas0?: string[] | null } = $props();

  // svelte-ignore state_referenced_locally
  let sets = $state<EventSetInfo[] | null>(sets0);
  // svelte-ignore state_referenced_locally
  let personas = $state<string[]>(personas0 ?? []);
  let setsError = $state<string | null>(null);
  // svelte-ignore state_referenced_locally
  let set = $state(sets0?.[0]?.code ?? '');
  let policies = $state<BotPolicyInfo[]>([]);
  let defaultPolicy = $state('');
  let botPolicy = $state<string[]>(Array(BOT_SEATS).fill(''));
  let botDrafter = $state<string[]>(Array(BOT_SEATS).fill(''));
  let busy = $state(false);
  let error = $state<string | null>(null);

  const offered = $derived(eventPolicies(policies));
  const chosen = $derived(sets?.find((s) => s.code === set) ?? null);
  const defaultLabel = $derived(policies.find((p) => p.name === defaultPolicy)?.label ?? defaultPolicy);

  onMount(() => {
    fetchBotPolicies()
      .then((l) => {
        policies = l.policies ?? [];
        defaultPolicy = l.default;
      })
      .catch(() => (policies = []));
    listEventSets()
      .then((l) => {
        sets = l ?? [];
        if (!sets.some((s) => s.code === set)) set = sets[0]?.code ?? '';
      })
      .catch((e) => {
        sets = [];
        setsError = message(e);
      });
    listEventPersonas()
      .then((p) => (personas = p.drafters ?? []))
      .catch(() => (personas = []));
  });

  function policyLabel(p: BotPolicyInfo): string {
    return p.tier && p.tier !== 'production' ? `${p.label} (${p.tier})` : p.label;
  }

  async function open() {
    if (!set) return;
    busy = true;
    error = null;
    try {
      const res = await createEvent(createRequest(set, botPolicy, botDrafter));
      const mine = res.invites?.[0];
      navigateTo(mine ? mine.join : `/e/${encodeURIComponent(res.event)}`);
    } catch (e) {
      error = message(e);
      busy = false;
    }
  }
</script>

<main class="ev" data-testid="new-event">
  <header class="ev-topbar">
    <a class="brand" href="/">gorge</a>
    <span class="crumbs">Events / New</span>
    <span class="sp"></span>
    <a class="ev-btn sm" href="/">Cancel</a>
  </header>
  <div class="ev-split create">
    <section class="ev-main form">
      <div class="field">
        <span class="ev-h5">Mode</span>
        <div class="seg" role="group" aria-label="Mode">
          <button type="button" class="on" aria-pressed="true">Draft</button>
          <button type="button" disabled>Sealed (later)</button>
        </div>
      </div>
      <div class="field">
        <span class="ev-h5">Seats · pod of 4</span>
        <div class="seats">
          <div class="seatrow">
            <span class="idx data">1</span>
            <b>You</b>
            <span class="data dim">human · you type your name next</span>
          </div>
          {#each botPolicy as _, i (i)}
            <div class="seatrow">
              <span class="idx data">{i + 2}</span>
              <span class="kind">Bot</span>
              <span class="persona">
                <select aria-label={`Seat ${i + 2} drafts as`} bind:value={botDrafter[i]} data-testid={`seat-${i + 2}-drafter`}>
                  <option value="">Drafter: server default ({DEFAULT_PERSONA})</option>
                  {#each personas as name (name)}
                    <option value={name}>Drafter: {name}</option>
                  {/each}
                </select>
                <span class="data dim">Builder: {botDrafter[i] || DEFAULT_PERSONA}</span>
              </span>
              <label class="plays">
                <span class="sr">Seat {i + 2} plays</span>
                <select aria-label={`Seat ${i + 2} plays`} bind:value={botPolicy[i]} data-testid={`seat-${i + 2}-policy`}>
                  <option value="">{defaultLabel ? `Plays: server default (${defaultLabel})` : 'Plays: server default'}</option>
                  {#each offered as p (p.name)}
                    <option value={p.name}>Plays: {policyLabel(p)}</option>
                  {/each}
                </select>
              </label>
            </div>
          {/each}
        </div>
        <p class="dim small">Only bots that cannot see your decklist are offered. Inviting more humans comes later.</p>
      </div>
      <div class="field">
        <span class="ev-h5">Matches</span>
        <div class="seg" role="group" aria-label="Matches">
          <button type="button" class="on" aria-pressed="true">Best of 1</button>
          <button type="button" disabled>Best of 3 (later)</button>
        </div>
        <p class="dim small">Round robin: three rounds, everyone plays everyone once.</p>
      </div>
    </section>
    <aside class="ev-rail">
      <span class="ev-h5">Set</span>
      <div class="setlist" role="radiogroup" aria-label="Set" data-testid="set-list">
        {#if sets === null}
          <p class="dim small">Loading the sets…</p>
        {:else}
          {#each sets as s (s.code)}
            <label class="setopt" class:on={set === s.code}>
              <input type="radio" name="event-set" value={s.code} bind:group={set} />
              <span class="code data">{s.code}</span>
              <span>{s.name || s.code}<small>{setNote(s)}</small></span>
            </label>
          {:else}
            <p class="dim small" role="status">{setsError ? `Could not load the sets: ${setsError}` : 'This server offers no draftable sets.'}</p>
          {/each}
        {/if}
      </div>
      <p class="dim small">Only sets where every booster card is fully supported are offered.</p>
      {#if chosen?.ratings === RATINGS_17LANDS}<Attribution what="Bot pick ratings" />{/if}
      <span class="sp"></span>
      {#if error}<p class="ev-error" role="alert">{error}</p>{/if}
      <button type="button" class="ev-btn primary" data-testid="open-pod" disabled={busy || !set} onclick={open}>{busy ? 'Opening…' : 'Open the pod'}</button>
    </aside>
  </div>
</main>

<style>
  .dim {
    color: var(--ink-dim);
  }
  .small {
    font-size: var(--t-12);
    margin: 0;
  }
  .form {
    gap: var(--sp-4);
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
  }
  .seg {
    display: inline-flex;
    flex-wrap: wrap;
    align-self: flex-start;
    border: 1px solid var(--edge-inst);
    border-radius: var(--radius);
    overflow: hidden;
  }
  .seg button {
    background: var(--instrument);
    border: 0;
    border-right: 1px solid var(--edge-inst);
    color: var(--ink-dim);
    padding: 0.4rem 0.8rem;
    font: 500 var(--t-14) var(--font-ui);
  }
  .seg button:last-child {
    border-right: 0;
  }
  .seg button.on {
    background: var(--felt-raised);
    color: var(--ink);
    box-shadow: inset 0 -2px 0 var(--gilt);
  }
  .seg button[disabled] {
    color: var(--ink-faint);
    cursor: not-allowed;
  }
  .seats {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .seatrow {
    display: grid;
    grid-template-columns: 1.5rem 3rem minmax(0, 1fr) minmax(0, 15rem);
    gap: var(--sp-2);
    align-items: center;
    padding: 0.45rem 0.6rem;
    background: var(--felt-raised);
    border: 1px solid var(--edge-felt);
    border-radius: var(--radius);
    font-size: var(--t-14);
  }
  .seatrow:first-child {
    grid-template-columns: 1.5rem 3rem minmax(0, 1fr);
  }
  .persona {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    font-size: var(--t-11);
  }
  .persona select {
    width: 100%;
  }
  .plays select {
    width: 100%;
  }
  .idx {
    color: var(--ink-faint);
  }
  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
  /* On a phone a bot row stacks: number and kind, then persona, then its select. */
  @media (max-width: 600px) {
    .seatrow {
      grid-template-columns: 1.5rem minmax(0, 1fr);
    }
    .seatrow:first-child {
      grid-template-columns: 1.5rem minmax(0, 1fr);
    }
    .persona,
    .plays {
      grid-column: 2;
    }
  }
  .setlist {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .setopt {
    display: grid;
    grid-template-columns: auto 2.75rem minmax(0, 1fr);
    gap: var(--sp-2);
    align-items: center;
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--edge-inst);
    border-radius: var(--radius);
    background: var(--instrument);
    cursor: pointer;
    font-size: var(--t-14);
  }
  .setopt input {
    accent-color: var(--gilt);
    margin: 0;
  }
  .setopt.on {
    border-color: var(--gilt);
    background: var(--instrument-raised);
  }
  .setopt small {
    display: block;
    color: var(--ink-dim);
    font-size: var(--t-11);
  }
  .code {
    color: var(--ink);
  }
  .sp {
    flex: 1;
  }
</style>

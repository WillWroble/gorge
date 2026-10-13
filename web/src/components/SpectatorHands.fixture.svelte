<script lang="ts">
  import { onMount } from 'svelte';
  import type { CardView, Decision, PaymentAction, PlayerView, SeatInfo, View } from '../protocol';
  import { castableActions } from '../lib/announcepay';
  import { optionsByObj, optionsByPlayer, type CardOptions } from '../lib/cardoptions';
  import { IMAGE_KEY } from '../lib/images';
  import { layoutStore } from '../lib/layouts.svelte';
  import { emptyLibrary } from '../lib/layoutlibrary';
  import { PRESET_IDS, type PresetId } from '../lib/layoutprofile';
  import BoardStage from './BoardStage.svelte';
  import HandFan from './HandFan.svelte';
  import Rail from './Rail.svelte';

  /**
   * SpectatorHands fixture: the whole table — BoardStage on the felt and the
   * Rail beside it, laid out the way Table.svelte lays them out — over a
   * representative board, so SpectatorHands.geometry.test.ts measures the
   * real passive hand fans against the real battlefield, centre strip, stack
   * and pending tray. No production caller.
   *
   * URL:
   *  - ?seats=2|4|8 (default 2) and ?preset=<layout preset> (optional);
   *  - ?mode=omni (default): an omniscient spectator, every hand exposed;
   *    mode=public: a public spectator, every hand redacted to JSON null
   *    (exactly what view.go sends; hand_size still says 7);
   *    mode=seated: seat 0's own client — own hand in the interactive
   *    HandFan with a CAST and a PLAY shortcut, the opponent redacted;
   *  - ?opphand=1 (seated only): the opponent's hand array is PRESENT in the
   *    payload anyway, to prove the seated gate is the seat, not the payload.
   *
   * window.__intents records what the seated fan posts; window.updateFixture
   * Hands() replaces the projection the way a live view update does.
   */

  const params = new URLSearchParams(location.search);
  const count = Math.min(8, Math.max(2, Number(params.get('seats') ?? '2') || 2));
  const mode = (params.get('mode') ?? 'omni') as 'omni' | 'public' | 'seated';
  const oppHand = params.get('opphand') === '1';
  layoutStore.replace(emptyLibrary());
  const preset = params.get('preset');
  if (preset !== null && (PRESET_IDS as readonly string[]).includes(preset)) layoutStore.applyPreset(preset as PresetId);

  const colours = ['#e5484d', '#30a46c', '#4a8fd4', '#d8a24a', '#a855f7', '#f97316', '#14b8a6', '#ec4899'];
  const card = (id: number, seat: number, name: string, types: string): CardView => {
    // "known no image": the blank face renders with no /art request.
    localStorage.setItem(`${IMAGE_KEY}${name}`, '');
    return {
      id, name, types, mana_cost: types.includes('Land') ? '' : '1 G', printing: { name }, token: '',
      tapped: false, power: types.includes('Creature') ? 2 : 0, toughness: types.includes('Creature') ? 2 : 0,
      damage: 0, attacking: false, controller: seat, owner: seat, summon_sick: false,
    };
  };
  const handOf = (seat: number, n = 7): CardView[] =>
    Array.from({ length: n }, (_, i) => card(1000 + seat * 100 + i, seat, `Seat${seat} Secret ${i + 1}`, i === 0 ? 'Land' : 'Creature — Elf'));
  const battlefieldOf = (seat: number): CardView[] => [
    ...Array.from({ length: 4 }, (_, i) => card(200 + seat * 20 + i, seat, `Forest ${seat}-${i}`, 'Basic Land — Forest')),
    ...Array.from({ length: 3 }, (_, i) => card(210 + seat * 20 + i, seat, `Bear ${seat}-${i}`, 'Creature — Bear')),
  ];
  const players: PlayerView[] = Array.from({ length: count }, (_, seat) => {
    const exposed = mode === 'omni' || (mode === 'seated' && (seat === 0 || oppHand));
    return {
      seat, name: `Player ${seat + 1}`, life: 20, lost: false, spells_cast_this_turn: 0, life_gained_this_turn: 0, life_lost_this_turn: 0, tokens_created_this_turn: 0, library_size: 50, hand_size: 7, graveyard_size: 0,
      completed_dungeons: 0,
      hand: exposed ? handOf(seat) : (null as unknown as CardView[]),
      pool: (mode === 'seated' && seat === 0 ? {} : null) as unknown as Record<string, number>,
      battlefield: battlefieldOf(seat), graveyard: [], exile: [], command: [], commanders: [], commander_casts: [],
    };
  });
  const seats: SeatInfo[] = players.map((p, seat) => ({ name: p.name, deck: 'fixture', colour: colours[seat] }));

  // The seated decision: one plan-less payment action for a hand creature and
  // a land play for the hand land — the CAST and PLAY shortcuts.
  const castAction: PaymentAction = { id: 'pay-fixture', cast: { object: 1001, face: 0, origin: 'hand' }, label: 'Cast Seat0 Secret 2', plans: [] };
  const decision: Decision = {
    seq: 5, player: 0, kind: 'priority', prompt: 'You have priority.', min: 1, max: 1,
    options: [
      { index: 3, kind: 'play_land', label: 'Play Seat0 Secret 1', player: 0, obj: 1000 },
      { index: 9, kind: 'pass', label: 'Pass priority', player: 0 },
    ],
    payment_actions: [castAction],
  };
  const intents: unknown[] = [];
  (window as unknown as { __intents: () => unknown[] }).__intents = () => JSON.parse(JSON.stringify(intents)) as unknown[];
  const options: CardOptions | null = mode === 'seated'
    ? { byObj: optionsByObj(decision), byPlayer: optionsByPlayer(decision), picked: [], tone: 'offered', post: (index: number) => intents.push({ post: index }) }
    : null;

  const initial: View = {
    viewer: mode === 'seated' ? 0 : 255,
    visibility: mode === 'omni' ? 'omniscient' : mode === 'public' ? 'public' : 'seat',
    turn: 3, round: 3, step: 'main1', phase: 'main1', active: 0, priority: 0,
    over: false, draw: false, winner: null,
    stack: [{ id: 900, kind: 'spell', name: 'Fixture Stack Spell', controller: 1, text: 'Draw a card.', targets: [], optional: false }],
    pending: [{ source: 901, controller: 1, label: 'Fixture pending trigger', optional: true, decider: 1 }],
    players,
    decision: mode === 'seated' ? decision : undefined,
  };
  let view = $state<View>(initial);
  const own = $derived(mode === 'seated' ? view.players[0] : null);

  onMount(() => {
    (window as Window & { updateFixtureHands?: () => void }).updateFixtureHands = () => {
      const next = view.players.map((p) => ({ ...p }));
      next[0].hand = [card(1099, 0, 'Seat0 Updated Card', 'Instant')];
      next[1].hand = [];
      view = { ...view, players: next };
    };
    (window as Window & { fixtureReady?: boolean }).fixtureReady = true;
  });
</script>

<main class="table">
  <section class="stage">
    <BoardStage {view} {seats} {options} seat={mode === 'seated' ? 0 : null} hand={own ? handFan : null} />
  </section>
  <aside class="rail">
    <Rail {view} {seats} decision={null} />
  </aside>
</main>

{#snippet handFan({ cardWidth, visible, raise }: { cardWidth: number; visible: number; raise: boolean })}
  {#if own}
    <HandFan player={own} {cardWidth} {visible} {raise} {options} paymentActions={castableActions(decision, true, true)} autoPay={true} onCastPayment={(action) => intents.push({ cast: action.id })} />
  {/if}
{/snippet}

<style>
  .table {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(17rem, 18%);
    width: 100vw;
    height: 100vh;
    background: var(--felt);
  }
  .stage {
    position: relative;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }
  .rail {
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    background: var(--instrument);
  }
</style>

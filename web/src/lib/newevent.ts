import type { BotPolicyInfo, CreateEventRequest } from '../protocol';

/**
 * The sets a pod can draft. The server validates the code (setdata.Load)
 * and offers no listing endpoint yet, so this is the client's copy of the
 * committed set specs (limited/setdata/sets/*.json); a code the server does
 * not know is refused with a 400 the create screen shows.
 */
export const EVENT_SETS: readonly { code: string; name: string; note: string }[] = [
  { code: 'FDN', name: 'Foundations', note: 'Play Booster · 14 cards · ratings: 17Lands' },
  { code: 'BLB', name: 'Bloomburrow', note: 'Play Booster · 14 cards · ratings: card heuristic' },
];

/** BOT_SEATS is the pod's bot count: E1 seats one human (seat 1) and three bots. */
export const BOT_SEATS = 3;

/**
 * eventPolicies narrows the server's offered bot policies to those an event
 * can seat. /api/bot-policies already lists only policies that cannot read
 * an opponent's decklist (bots.Info.Env == false); an event game is a
 * constructed-format table, so a policy must play constructed. The server
 * re-checks every name at create time.
 */
export function eventPolicies(list: BotPolicyInfo[]): BotPolicyInfo[] {
  return list.filter((p) => p.formats.includes('constructed'));
}

/** createRequest builds the POST /api/events body: seat 0 is the human, then the bots with their chosen policies ("" = server default). */
export function createRequest(set: string, botPolicies: string[]): CreateEventRequest {
  return {
    set,
    seats: [
      { kind: 'human' },
      ...botPolicies.map((p) => (p ? { kind: 'bot', policy: p } : { kind: 'bot' })),
    ],
  };
}

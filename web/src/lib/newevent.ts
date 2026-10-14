import type { BotPolicyInfo, CreateEventRequest, EventSetInfo } from '../protocol';

/** BOT_SEATS is the pod's bot count: E1 seats one human (seat 1) and three bots. */
export const BOT_SEATS = 3;

/** DEFAULT_PERSONA is the pickbot persona the server seats when a create body names none (eventhost: Drafter "" = "rating-colour"). */
export const DEFAULT_PERSONA = 'rating-colour';

/** RATINGS_17LANDS is EventSetInfo.ratings for a set whose pick ratings come from 17Lands' public data. */
export const RATINGS_17LANDS = '17lands';

/**
 * setNote is the line under a set's name on the create screen: the server's
 * pack size and rating source. Only a 17lands-rated set names 17Lands.
 */
export function setNote(s: EventSetInfo): string {
  const ratings = `ratings: ${s.ratings === RATINGS_17LANDS ? '17Lands' : 'card heuristic'}`;
  return s.pack_size > 0 ? `${s.pack_size}-card packs · ${ratings}` : ratings;
}

/**
 * ratedBy17Lands reports whether a set's bot ratings come from 17Lands, so
 * the screens owe its CC BY credit. Unknown — the catalogue not loaded, or
 * a set it does not list — keeps the credit: attribution the data may
 * need is never dropped on a guess.
 */
export function ratedBy17Lands(sets: EventSetInfo[] | null, code: string | undefined): boolean {
  const s = sets?.find((x) => x.code === code);
  return !s || s.ratings === RATINGS_17LANDS;
}

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

/**
 * createRequest builds the POST /api/events body: seat 0 is the human, then
 * the bots with their chosen game policies and pickbot personas ("" = the
 * server's default for either). It never carries a seed: the server mints
 * it, and an unknown body field is a 400.
 */
export function createRequest(set: string, botPolicies: string[], drafters: string[] = []): CreateEventRequest {
  return {
    set,
    seats: [
      { kind: 'human' },
      ...botPolicies.map((p, i) => {
        const seat: CreateEventRequest['seats'][number] = { kind: 'bot' };
        if (p) seat.policy = p;
        if (drafters[i]) seat.drafter = drafters[i];
        return seat;
      }),
    ],
  };
}

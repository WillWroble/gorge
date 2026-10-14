import type { EventSeries } from '../protocol';

/**
 * series.ts renders an EventSeries row's facts for the hub. Every value is
 * the server's: the score, the per-game winners (`games`, -1 = drawn), the
 * live or next game number and the between-games state. Nothing here
 * decides a result.
 */

/** GameDot is one played game of a series from one side: won, lost or drawn. */
export type GameDot = 'w' | 'l' | 'd';

/** gameDots reads the series' per-game winners from `side`'s point of view. */
export function gameDots(s: EventSeries, side: number): GameDot[] {
  return (s.games ?? []).map((w) => (w < 0 ? 'd' : w === side ? 'w' : 'l'));
}

/** seriesScore is the row's score ("2 – 1"), or "vs" before the series starts. */
export function seriesScore(s: EventSeries): string {
  if (s.state === 'pending') return 'vs';
  return `${s.score?.[0] ?? 0} – ${s.score?.[1] ?? 0}`;
}

/**
 * seriesStatus is the small line under the score: pending, live (with the
 * game number for a best of 3, and the attempt when a game was retried),
 * between games, or the final outcome.
 */
export function seriesStatus(s: EventSeries): string {
  switch (s.state) {
    case 'live': {
      const parts = ['live'];
      if (s.best_of > 1 && s.game) parts.push(`game ${s.game}`);
      if (s.attempt && s.attempt > 1) parts.push(`attempt ${s.attempt}`);
      return parts.join(' · ');
    }
    case 'between':
      return 'between games';
    case 'ended':
      return s.winner === null || s.winner === undefined ? 'draw' : 'final';
    default:
      return 'pending';
  }
}

/** inPlay reports a series whose games are under way: live, or between two of its games. */
export const inPlay = (s: EventSeries): boolean => s.state === 'live' || s.state === 'between';

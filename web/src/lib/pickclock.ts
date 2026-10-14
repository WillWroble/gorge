/**
 * pickclock.ts is the draft screen's pick countdown arithmetic. The
 * deadline is the server's: a draft_pack carries `remaining_ms` as of the
 * moment it was sent, and the client only counts that down locally from
 * the moment the frame arrived. At zero the client shows "auto-picking…"
 * and waits — the server makes the pick and sends the next pack, which
 * restarts the count.
 */

/** WARN_SECONDS is where the countdown turns to its warning style. */
export const WARN_SECONDS = 10;

/** pickSecondsLeft is the whole seconds left on a pick, never below zero. */
export function pickSecondsLeft(remainingMs: number, arrivedAt: number, now: number): number {
  const left = remainingMs - Math.max(0, now - arrivedAt);
  return left <= 0 ? 0 : Math.ceil(left / 1000);
}

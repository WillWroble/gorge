import { CARD_RATIO } from './cardsizing';

/**
 * passivehand is the pure sizing behind a SPECTATOR's passive hand fans: an
 * omniscient spectator sees every hand the server exposes drawn as real card
 * faces in that player's own seat panel, as if sitting at the table with every
 * hand face up (inbox-omniscient-rendered-hand-fans).
 *
 * The fan takes a band of the seat panel AWAY from the battlefield rather than
 * floating over it: Board subtracts the band from the panel before sizing the
 * seat's rows, so a hand can never cover a permanent. A full seat panel puts
 * the band along its rim (the top edge for a seat across the table, the bottom
 * edge for the seat drawn at the bottom); a focus side strip is too short for a
 * second row, so its band sits at the strip's right end instead and takes width.
 *
 * Every full-size panel's fan shares ONE face height (the smallest any of them
 * asks for), the same "the table reads evenly" rule the battlefield cards use.
 * No DOM: Board does the arithmetic once per layout.
 */

/** px of breathing room around the faces inside the band, each side */
export const PASSIVE_PAD = 3;
/** px between two faces when the hand fits without overlapping */
export const PASSIVE_GAP = 4;
/** the share of a full seat panel's height a passive fan's face may take */
export const PASSIVE_SHARE = 0.22;
/** a passive face is never shorter than this in a full panel: below it a face stops reading as a card */
export const PASSIVE_FACE_MIN = 40;
/** nor taller than this: the fan is a reading aid, the battlefield is the board */
export const PASSIVE_FACE_MAX = 104;
/** a focus side strip's fan may take at most this share of the strip's width */
export const PASSIVE_STRIP_W_SHARE = 0.4;
/** the smallest face a side strip draws */
export const PASSIVE_STRIP_FACE_MIN = 20;

/** passiveFaceH is the face height a full seat panel of panelH px asks for. */
export function passiveFaceH(panelH: number): number {
  return Math.round(Math.min(PASSIVE_FACE_MAX, Math.max(PASSIVE_FACE_MIN, panelH * PASSIVE_SHARE)));
}

/** passiveBandH is the height a rim band holding faceH-tall faces takes from the panel. */
export function passiveBandH(faceH: number): number {
  return faceH + PASSIVE_PAD * 2;
}

/** passiveFaceW is the width of one face faceH px tall. */
export function passiveFaceW(faceH: number): number {
  return Math.max(12, Math.round(faceH * CARD_RATIO));
}

export interface StripHand {
  /** one face's height, px */
  faceH: number;
  /** the band's width taken from the strip's rows, px */
  bandW: number;
}

/**
 * stripHand sizes a focus side strip's fan: the faces are as tall as the
 * strip's body (bodyH), and the band is as wide as the hand laid edge to edge
 * would be, capped at PASSIVE_STRIP_W_SHARE of the strip so the creatures row
 * keeps the rest; a hand that does not fit overlaps inside that cap.
 */
export function stripHand(panelW: number, bodyH: number, count: number): StripHand {
  const faceH = Math.max(PASSIVE_STRIP_FACE_MIN, Math.round(bodyH - PASSIVE_PAD * 2));
  const faceW = passiveFaceW(faceH);
  const natural = Math.max(1, count) * faceW + Math.max(0, count - 1) * PASSIVE_GAP + PASSIVE_PAD * 2;
  const cap = Math.round(panelW * PASSIVE_STRIP_W_SHARE);
  return { faceH, bandW: Math.min(natural, Math.max(faceW + PASSIVE_PAD * 2, cap)) };
}

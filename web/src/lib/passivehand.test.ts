import { describe, expect, it } from 'vitest';
import { CARD_RATIO } from './cardsizing';
import { PASSIVE_FACE_MAX, PASSIVE_FACE_MIN, PASSIVE_PAD, PASSIVE_STRIP_FACE_MIN, PASSIVE_STRIP_W_SHARE, passiveBandH, passiveFaceH, passiveFaceW, stripHand } from './passivehand';

describe('passivehand sizing', () => {
  it('a full panel face is a share of the panel height, clamped to a readable card', () => {
    expect(passiveFaceH(400)).toBe(88);
    expect(passiveFaceH(100)).toBe(PASSIVE_FACE_MIN);
    expect(passiveFaceH(2000)).toBe(PASSIVE_FACE_MAX);
  });

  it('the rim band is one face plus its padding; a face keeps the card ratio', () => {
    expect(passiveBandH(80)).toBe(80 + PASSIVE_PAD * 2);
    expect(passiveFaceW(88)).toBe(Math.round(88 * CARD_RATIO));
  });

  it('a side strip fan is as tall as the strip body and never takes more than its share of the width', () => {
    const small = stripHand(400, 40, 2);
    expect(small.faceH).toBe(40 - PASSIVE_PAD * 2);
    // two faces edge to edge fit under the cap: the band is exactly that wide
    expect(small.bandW).toBe(2 * passiveFaceW(small.faceH) + 4 + PASSIVE_PAD * 2);
    const big = stripHand(400, 40, 12);
    expect(big.bandW).toBe(Math.round(400 * PASSIVE_STRIP_W_SHARE));
    expect(stripHand(400, 5, 3).faceH).toBe(PASSIVE_STRIP_FACE_MIN);
  });
});

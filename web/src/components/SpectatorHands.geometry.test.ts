import { type Browser, type Page } from 'playwright';
import { beforeAll, describe, expect, it } from 'vitest';
import { browserURL, sharedBrowser } from '../test/browser';

/**
 * SpectatorHands.geometry.test.ts is the browser proof for the spectator's
 * passive hand fans (inbox-omniscient-rendered-hand-fans): an omniscient
 * spectator sees every exposed hand as card faces in that player's own seat
 * panel; a public spectator sees no hand identity at all; a seated player
 * keeps the interactive own-hand fan and gains no opponent hand.
 *
 * It mounts the REAL BoardStage and Rail side by side (SpectatorHands.fixture)
 * and measures the rendered DOM — the band is sized by arithmetic, but only a
 * browser can say whether a face actually lands on a permanent, the centre
 * strip, the stack or the pending tray.
 */

let browser: Browser;

beforeAll(async () => {
  browser = await sharedBrowser();
});

interface Rect { top: number; bottom: number; left: number; right: number; width: number; height: number }

const TOL = 0.5;

function intersects(a: Rect, b: Rect): boolean {
  return a.left < b.right - TOL && b.left < a.right - TOL && a.top < b.bottom - TOL && b.top < a.bottom - TOL;
}
function inside(inner: Rect, outer: Rect): boolean {
  return inner.left >= outer.left - TOL && inner.right <= outer.right + TOL && inner.top >= outer.top - TOL && inner.bottom <= outer.bottom + TOL;
}

async function open(query: string, width: number, height: number): Promise<Page> {
  const page = await browser.newPage({ viewport: { width, height } });
  await page.goto(`${browserURL}src/components/SpectatorHands.fixture.html?${query}`);
  await page.waitForFunction(() => (window as Window & { fixtureReady?: boolean }).fixtureReady === true, undefined, { timeout: 60_000 });
  await page.waitForSelector('[data-own-board] .quadrant', { state: 'attached', timeout: 60_000 });
  return page;
}

interface Fan {
  seat: number;
  panelSeat: number | null;
  fan: Rect;
  panel: Rect | null;
  faces: { obj: number; rect: Rect; image: boolean }[];
  controls: number;
}

async function measure(page: Page) {
  return page.evaluate(() => {
    const r = (el: Element): Rect => {
      const b = el.getBoundingClientRect();
      return { top: b.top, bottom: b.bottom, left: b.left, right: b.right, width: b.width, height: b.height };
    };
    const fans: Fan[] = [...document.querySelectorAll('[data-passive-hand]')].map((el) => {
      const panel = el.closest('.quadrant');
      return {
        seat: Number(el.getAttribute('data-passive-hand')),
        panelSeat: panel ? Number(panel.getAttribute('data-seat')) : null,
        fan: r(el),
        panel: panel ? r(panel) : null,
        faces: [...el.querySelectorAll('[data-hand-card]')].map((c) => ({
          obj: Number(c.getAttribute('data-hand-card')),
          rect: r(c),
          image: c.querySelector('.card-image') !== null,
        })),
        // anything a seat could act through: a real button, a menu, the
        // option/payment/land-play affordances HandFan carries.
        controls: el.querySelectorAll('button, [role="button"], [role="menu"], [role="menuitem"], [aria-haspopup], [data-payment-card], [data-play-land], [data-options], [data-tone], .tile-actions, .payment-shortcut').length,
      };
    });
    const q = (s: string) => document.querySelector(s);
    return {
      fans,
      stage: r(q('[data-board-stage]')!),
      strip: r(q('[data-centre-strip]')!),
      stack: r(q('.rail section.stack')!),
      pending: r(q('.rail section.pending')!),
      rail: r(q('.rail')!),
      tiles: [...document.querySelectorAll('.quadrant .card-tile')].map(r),
      quadrants: document.querySelectorAll('.quadrant').length,
      railText: q('.rail')!.textContent ?? '',
    };
  });
}

/** assertTable runs every layout invariant one omniscient table must keep. */
function assertTable(m: Awaited<ReturnType<typeof measure>>, seats: number, at: string): void {
  expect(m.quadrants, `${at}: every seat drawn`).toBe(seats);
  expect(m.fans.map((f) => f.seat).sort((a, b) => a - b), `${at}: every exposed hand rendered`).toEqual(Array.from({ length: seats }, (_, i) => i));
  for (const f of m.fans) {
    const who = `${at} seat ${f.seat}`;
    // In THAT player's own seat panel, and inside it.
    expect(f.panelSeat, `${who}: fan sits in its own seat panel`).toBe(f.seat);
    expect(inside(f.fan, f.panel!), `${who}: fan ${JSON.stringify(f.fan)} inside panel ${JSON.stringify(f.panel)}`).toBe(true);
    // Rendered card faces, not a names list.
    expect(f.faces, `${who}: seven faces`).toHaveLength(7);
    for (const face of f.faces) {
      expect(face.image, `${who}: face ${face.obj} draws a CardImage`).toBe(true);
      expect(face.rect.height, `${who}: face ${face.obj} has a card's height`).toBeGreaterThanOrEqual(20);
      expect(face.rect.width / face.rect.height, `${who}: face ${face.obj} has a card's shape`).toBeCloseTo(63 / 88, 1);
      expect(inside(face.rect, f.fan), `${who}: face ${face.obj} inside its band`).toBe(true);
      for (const t of m.tiles) expect(intersects(face.rect, t), `${who}: face ${face.obj} covers a permanent ${JSON.stringify(t)}`).toBe(false);
    }
    expect(f.controls, `${who}: a passive fan carries no control`).toBe(0);
    // Never over the phase controls, the stack, the pending tray or the rail.
    expect(intersects(f.fan, m.strip), `${who}: fan over the centre strip`).toBe(false);
    expect(intersects(f.fan, m.stack), `${who}: fan over the stack`).toBe(false);
    expect(intersects(f.fan, m.pending), `${who}: fan over the pending tray`).toBe(false);
    expect(f.fan.right, `${who}: fan stays on the felt`).toBeLessThanOrEqual(m.stage.right + TOL);
    for (const g of m.fans) if (g !== f) expect(intersects(f.fan, g.fan), `${who}: overlaps seat ${g.seat}'s fan`).toBe(false);
  }
  // The rail keeps its summary, stack and pending, and no longer lists hands.
  expect(m.railText, `${at}: rail lists no hand identity`).not.toContain('Secret');
  expect(m.railText).toContain('Fixture Stack Spell');
  expect(m.railText).toContain('Fixture pending trigger');
}

const VIEWPORTS: [number, number][] = [[1440, 900], [1000, 900], [650, 700]];

describe('Spectator hands — an omniscient spectator sees every hand as card faces at its seat', () => {
  it('two seats: both hands render as faces in their own panels at every acceptance viewport', { timeout: 120_000 }, async () => {
    for (const [w, h] of VIEWPORTS) {
      const page = await open('seats=2&mode=omni', w, h);
      try {
        assertTable(await measure(page), 2, `2 seats ${w}x${h}`);
        // The seat across the table holds its hand at its rim (top of its
        // panel), the seat at the bottom at the bottom of its panel.
        const m = await measure(page);
        const top = m.fans.find((f) => f.seat === 1)!;
        const bottom = m.fans.find((f) => f.seat === 0)!;
        expect(top.fan.bottom).toBeLessThanOrEqual(m.strip.top + TOL);
        expect(bottom.fan.top).toBeGreaterThanOrEqual(m.strip.bottom - TOL);
      } finally {
        await page.close();
      }
    }
  });

  it('four seats: every exposed hand renders without overlap or controls (columns and the cmd4 grid)', { timeout: 120_000 }, async () => {
    for (const [w, h] of VIEWPORTS) {
      for (const q of ['seats=4&mode=omni', 'seats=4&mode=omni&preset=cmd4']) {
        const page = await open(q, w, h);
        try {
          assertTable(await measure(page), 4, `${q} ${w}x${h}`);
        } finally {
          await page.close();
        }
      }
    }
  });

  it('eight seats in focus: a side strip compacts its fan but still draws card faces', { timeout: 60_000 }, async () => {
    const page = await open('seats=8&mode=omni&preset=focus8', 1440, 900);
    try {
      assertTable(await measure(page), 8, 'focus8 1440x900');
    } finally {
      await page.close();
    }
  });

  it('hover and keyboard focus open the shared CardDetail inspector, read-only; Escape closes it', async () => {
    const page = await open('seats=2&mode=omni', 1440, 900);
    try {
      const face = page.locator('[data-passive-hand="1"] [data-hand-card="1102"] .face');
      await face.hover();
      await page.waitForSelector('body > #card-detail-1102', { timeout: 5_000 });
      expect(await page.locator('body > #card-detail-1102').textContent()).toContain('Seat1 Secret 3');
      await page.mouse.move(2, 2);
      await page.waitForSelector('body > #card-detail-1102', { state: 'detached' });

      await face.focus();
      await page.waitForSelector('body > #card-detail-1102');
      expect(await face.getAttribute('aria-describedby')).toBe('card-detail-1102');
      await page.keyboard.press('Escape');
      await page.waitForSelector('body > #card-detail-1102', { state: 'detached' });

      // A click is not an action: nothing is offered and nothing changes.
      await face.click();
      expect(await page.locator('[data-passive-hand] button, [role="menu"]').count()).toBe(0);
    } finally {
      await page.close();
    }
  });

  it('follows a live view update: a changed hand re-renders, an emptied hand keeps its empty band', async () => {
    const page = await open('seats=2&mode=omni', 1440, 900);
    try {
      await page.evaluate(() => (window as Window & { updateFixtureHands?: () => void }).updateFixtureHands?.());
      await page.waitForFunction(() => document.querySelector('[data-passive-hand="0"]')?.textContent?.includes('Seat0 Updated Card'));
      expect(await page.locator('[data-passive-hand="0"] [data-hand-card]').count()).toBe(1);
      expect(await page.locator('[data-passive-hand="0"]').textContent()).not.toContain('Seat0 Secret');
      expect(await page.locator('[data-passive-hand="1"] [data-hand-card]').count()).toBe(0);
      expect(await page.locator('[data-passive-hand="1"]').textContent()).toContain('No cards in hand');
    } finally {
      await page.close();
    }
  });
});

describe('Spectator hands — a public spectator sees no hand identity', () => {
  it('renders no fan, name, image or object id for any redacted hand, while the counts stay', { timeout: 60_000 }, async () => {
    for (const seats of [2, 4]) {
      const page = await open(`seats=${seats}&mode=public`, 1440, 900);
      try {
        const m = await page.evaluate(() => ({
          fans: document.querySelectorAll('[data-passive-hand], .passive-hand, .handfan').length,
          html: document.documentElement.outerHTML,
          // hand object ids are 1000-1799 in the fixture
          handObjs: [...document.querySelectorAll('[data-obj]')].map((e) => Number(e.getAttribute('data-obj'))).filter((n) => n >= 1000),
          imgs: [...document.querySelectorAll('img')].map((i) => i.alt).filter((a) => a.includes('Secret')),
          handCounts: document.querySelectorAll('[data-seat-header]').length,
        }));
        expect(m.fans, `${seats} seats: no hand fan`).toBe(0);
        expect(m.html, `${seats} seats: no hidden card name`).not.toContain('Secret');
        expect(m.handObjs, `${seats} seats: no hidden object id`).toEqual([]);
        expect(m.imgs, `${seats} seats: no hidden card image`).toEqual([]);
        expect(m.handCounts, `${seats} seats: seats still drawn`).toBe(seats);
        expect(await page.locator('[data-seat-row="1"]').textContent()).toContain('7');
      } finally {
        await page.close();
      }
    }
  });
});

describe('Spectator hands — a seated player keeps the interactive own hand only', () => {
  for (const opp of ['', '&opphand=1']) {
    it(`own HandFan keeps CAST and PLAY; no opponent hand is introduced${opp ? ' (even with an opponent hand in the payload)' : ''}`, async () => {
      const page = await open(`seats=2&mode=seated${opp}`, 1440, 900);
      try {
        await page.waitForSelector('.handtrack .handfan');
        expect(await page.locator('.handtrack .handfan [data-obj]').count()).toBe(7);
        expect(await page.locator('[data-passive-hand]').count()).toBe(0);
        expect(await page.evaluate(() => document.documentElement.outerHTML.includes('Seat1 Secret'))).toBe(false);

        const cast = page.locator('[data-payment-card="pay-fixture"]');
        const play = page.locator('[data-play-land="3"]');
        expect(await cast.count()).toBe(1);
        expect(await play.count()).toBe(1);
        await cast.click();
        await play.click();
        await expect.poll(() => page.evaluate(() => (window as unknown as { __intents: () => unknown[] }).__intents())).toEqual([{ cast: 'pay-fixture' }, { post: 3 }]);
      } finally {
        await page.close();
      }
    });
  }
});

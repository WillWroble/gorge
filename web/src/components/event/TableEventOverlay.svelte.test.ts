import { beforeAll, describe, expect, it } from 'vitest';
import type { Browser } from 'playwright';
import { browserURL, sharedBrowser } from '../../test/browser';

// The table page's event layer in a real browser: it loads the event with
// the REMEMBERED event seat (never the URL's game token), overlays the
// between-games panel, and reports the series in play — the binding that
// keeps Table.svelte on the finished board between two games (the client's
// table store guesses "idle" at a non-perpetual table's match_end; a series
// table goes on to its next match).
describe('TableEventOverlay (browser)', () => {
  let browser: Browser;
  beforeAll(async () => {
    browser = await sharedBrowser();
  });

  it('between games: the panel overlays the table, inPlay is true, and the event token is the remembered one', async () => {
    const page = await browser.newPage();
    await page.goto(`${browserURL}src/components/event/TableEventOverlay.fixture.html?state=between`);
    await page.getByTestId('table-between').waitFor();
    await expect.poll(() => page.getByTestId('in-play').textContent()).toBe('true');
    expect(await page.getByTestId('sb-chooser').textContent()).toBe('You will choose play or draw');
    const auths = await page.evaluate(() => (window as unknown as { auths: string[] }).auths);
    expect(auths.length).toBeGreaterThan(0);
    expect(auths.every((a) => a === 'Bearer EVENTtok0')).toBe(true);
    // Folding the panel shows the board, with a way back.
    await page.getByRole('button', { name: 'Show the board' }).click();
    await page.getByTestId('table-between-folded').waitFor();
    await page.getByRole('button', { name: 'Sideboard' }).click();
    await page.getByTestId('table-between').waitFor();
    await page.close();
  });

  it('a live game: no overlay, and inPlay stays true until the series ends', async () => {
    const page = await browser.newPage();
    await page.goto(`${browserURL}src/components/event/TableEventOverlay.fixture.html?state=live`);
    await expect.poll(() => page.getByTestId('in-play').textContent()).toBe('true');
    expect(await page.getByTestId('table-between').count()).toBe(0);
    await page.close();
  });
});

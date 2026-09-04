import {expect, test, type Page} from '@playwright/test';

/** The mock table. `id` is the fixture room; `scenario` picks the frame. */
const TABLE = '/table?id=01ARZ3NDEKTSV4RRFFQ69G5FAV&scenario=';

/** Portrait handsets first (the stage-v capsule), then the widths where the
 * layout swaps to the desktop oval. 320x568 is the narrowest phone still in
 * the wild and the tier that clipped first; 390x844 is the iPhone 15/16e class
 * the streak badge was reported cut on; 430x932 is the Pro Max class. */
const VIEWPORTS = [
  {name: '320x568', width: 320, height: 568},
  {name: '390x844', width: 390, height: 844},
  {name: '430x932', width: 430, height: 932},
  {name: '768x1024', width: 768, height: 1024},
  {name: '1280x800', width: 1280, height: 800},
  {name: '1440x900', width: 1440, height: 900},
];

/** Occupancy and street both move seats, cards and chips. */
const SCENARIOS = ['heads_up', 'flop', 'nine_max', 'showdown'];

async function openTable(page: Page, scenario: string) {
  await page.goto(`${TABLE}${scenario}`);
  await page.waitForSelector('.game', {state: 'attached'});
  // The deal/reveal animations move cards; assert the settled layout.
  await page.waitForTimeout(1200);
}

/** Every element that a clipping ancestor cuts off, with how far past which
 * edge it went. Runs in the page: jsdom cannot answer this at all.
 *
 * "Clipping ancestor" is resolved per element rather than assumed, because the
 * portrait stage clips (`.game-table.stage-v { overflow: hidden }`) while the
 * desktop oval deliberately lets seats sit outside the felt's box. */
async function clippedElements(page: Page) {
  return page.evaluate(() => {
    const clipperOf = (el: Element) => {
      for (let node = el.parentElement; node; node = node.parentElement) {
        const {overflowX, overflowY} = getComputedStyle(node);
        if (overflowX !== 'visible' || overflowY !== 'visible') return node;
      }
      return null;
    };
    const out: {selector: string; past: string; by: number}[] = [];
    // Seat badges and cards are the payload; the decorative felt washes and
    // glows are allowed to bleed past their box on purpose.
    const targets = '.game-seat, .seat-role, .seat-streak, .seat-timebank-badge, .playing-card, .seat-info, .seat-bet';
    for (const el of document.querySelectorAll(targets)) {
      const clipper = clipperOf(el);
      if (!clipper) continue;
      const r = el.getBoundingClientRect();
      if (!r.width && !r.height) continue;
      if (getComputedStyle(el).visibility === 'hidden') continue;
      const c = clipper.getBoundingClientRect();
      const edges: [string, number][] = [
        ['left', c.left - r.left], ['right', r.right - c.right],
        ['top', c.top - r.top], ['bottom', r.bottom - c.bottom],
      ];
      for (const [past, by] of edges) {
        // Sub-pixel rounding is not a clip; half a badge is.
        if (by > 1) out.push({selector: el.className.toString().trim().slice(0, 60), past, by: Math.round(by)});
      }
    }
    return out;
  });
}

for (const viewport of VIEWPORTS) {
  test.describe(`table at ${viewport.name}`, () => {
    test.use({viewport: {width: viewport.width, height: viewport.height}});

    for (const scenario of SCENARIOS) {
      test(`${scenario}: nothing a player needs is clipped`, async ({page}) => {
        await openTable(page, scenario);
        expect(await clippedElements(page)).toEqual([]);
      });
    }

    test('the page never scrolls sideways', async ({page}) => {
      await openTable(page, 'flop');
      const overflow = await page.evaluate(() => {
        const root = document.documentElement;
        return root.scrollWidth - root.clientWidth;
      });
      expect(overflow).toBeLessThanOrEqual(0);
    });

    // The streak badge is the reported case: it hangs 8px below and left of the
    // seat box, and the docked viewer seat is the portrait stage's last flex
    // child, so it lands exactly on the clip edge. Asserted with the badge
    // forced on, because the mock fixtures carry no streak.
    test('the win/loss streak badge fits inside the stage', async ({page}) => {
      await openTable(page, 'flop');
      const fits = await page.evaluate(() => {
        const seat = document.querySelector('.game-seat.viewer');
        if (!seat) return null;
        const badge = document.createElement('span');
        badge.className = 'seat-streak is-hot';
        badge.textContent = 'V3';
        seat.appendChild(badge);
        const b = badge.getBoundingClientRect();
        let clipper: Element | null = null;
        for (let node = seat.parentElement; node; node = node.parentElement) {
          const {overflowX, overflowY} = getComputedStyle(node);
          if (overflowX !== 'visible' || overflowY !== 'visible') {
            clipper = node;
            break;
          }
        }
        const c = clipper!.getBoundingClientRect();
        badge.remove();
        return {
          left: Math.round(b.left - c.left), bottom: Math.round(c.bottom - b.bottom),
          width: Math.round(b.width),
        };
      });
      expect(fits).not.toBeNull();
      expect(fits!.width).toBeGreaterThan(0);
      expect(fits!.left).toBeGreaterThanOrEqual(0);
      expect(fits!.bottom).toBeGreaterThanOrEqual(0);
    });
  });
}

// Engine-ordering regressions, not layout: the toggle's click used to race the
// hover the same pointer movement performed, and Firefox resolved it the other
// way from Chromium, so the reactions panel never opened on the first click.
test.describe('table asides', () => {
  test.use({viewport: {width: 1440, height: 900}});

  test('the reactions toggle opens on the first click and stays open', async ({page}) => {
    await openTable(page, 'flop');
    const aside = page.locator('.table-reactions');
    const toggle = aside.locator('.reaction-toggle').first();

    await toggle.click();
    await expect(aside).toHaveClass(/open/);
    // The regression was not a missing open, it was an open followed
    // immediately by the same gesture's close, so the panel has to survive the
    // renders that land after the click.
    await page.waitForTimeout(600);
    await expect(aside).toHaveClass(/open/);
    await expect(aside.locator('.reaction-panel')).toBeVisible();
  });

  // Leaving the aside still closes it: the pin must not turn the hover panel
  // into one that can never be dismissed.
  test('the panel closes once the pointer leaves', async ({page}) => {
    await openTable(page, 'flop');
    const aside = page.locator('.table-reactions');
    await aside.locator('.reaction-toggle').first().click();
    await expect(aside).toHaveClass(/open/);
    await page.mouse.move(10, 10);
    await expect(aside).not.toHaveClass(/open/);
  });
});

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
  // Phone held sideways: the two-column stage-h layout (table left, viewer +
  // dock right). `.game` clips, so nothing on the ring may leave the viewport.
  {name: '812x375', width: 812, height: 375},
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

    // The walnut band is the felt's inset minus the rail's, and the felt is
    // derived from the rail plus one --table-rail-band. Anything that reads
    // unequal here means someone tuned a single side again — which is how a
    // heads-up table ended up with a 63px rim at the sides and 14px at the
    // bottom.
    for (const scenario of ['heads_up', 'nine_max']) {
      test(`${scenario}: the rail band is the same thickness all the way round`, async ({page}) => {
        await openTable(page, scenario);
        const band = await page.evaluate(() => {
          const felt = document.querySelector('.game-felt')!;
          const rail = document.querySelector('.game-rail');
          // stage-h: the walnut is the felt's own border (a constant-width
          // stroke), the separate .game-rail ellipse is display:none.
          if (!rail || getComputedStyle(rail).display === 'none') {
            const cs = getComputedStyle(felt);
            return {
              top: Math.round(parseFloat(cs.borderTopWidth)),
              right: Math.round(parseFloat(cs.borderRightWidth)),
              bottom: Math.round(parseFloat(cs.borderBottomWidth)),
              left: Math.round(parseFloat(cs.borderLeftWidth)),
            };
          }
          const r = rail.getBoundingClientRect();
          const f = felt.getBoundingClientRect();
          return {
            top: Math.round(f.top - r.top), right: Math.round(r.right - f.right),
            bottom: Math.round(r.bottom - f.bottom), left: Math.round(f.left - r.left),
          };
        });
        const sides = Object.values(band);
        expect(Math.min(...sides)).toBeGreaterThan(0);
        // One pixel of rounding on a subpixel box is not an asymmetry.
        expect(Math.max(...sides) - Math.min(...sides)).toBeLessThanOrEqual(1);
      });
    }

    // Seats are published as normalised orbit coordinates and placed against
    // the same rail tokens, so their centres are on the band's centreline by
    // construction. This is the assertion that keeps the three geometries —
    // rail, felt and seat ring — from drifting apart again.
    test('every balanced seat sits on the band centreline', async ({page}) => {
      await openTable(page, 'nine_max');
      const worst = await page.evaluate(() => {
        const feltEl = document.querySelector('.game-felt')!;
        const felt = feltEl.getBoundingClientRect();
        const railEl = document.querySelector('.game-rail');
        let mid;
        if (!railEl || getComputedStyle(railEl).display === 'none') {
          // stage-h: centreline is the felt's border centre.
          const cs = getComputedStyle(feltEl);
          const bt = parseFloat(cs.borderTopWidth) / 2, br = parseFloat(cs.borderRightWidth) / 2;
          const bb = parseFloat(cs.borderBottomWidth) / 2, bl = parseFloat(cs.borderLeftWidth) / 2;
          mid = {left: felt.left + bl, right: felt.right - br, top: felt.top + bt, bottom: felt.bottom - bb};
        } else {
          const rail = railEl.getBoundingClientRect();
          mid = {
            left: (rail.left + felt.left) / 2, right: (rail.right + felt.right) / 2,
            top: (rail.top + felt.top) / 2, bottom: (rail.bottom + felt.bottom) / 2,
          };
        }
        return [...document.querySelectorAll('.game-seat[data-balanced-seat]')].reduce((max, seat) => {
          const box = seat.getBoundingClientRect();
          const style = getComputedStyle(seat);
          const s = Number(style.getPropertyValue('--seat-s'));
          const t = Number(style.getPropertyValue('--seat-t'));
          const dx = box.x + box.width / 2 - (mid.left + s * (mid.right - mid.left));
          const dy = box.y + box.height / 2 - (mid.top + t * (mid.bottom - mid.top));
          return Math.max(max, Math.abs(dx), Math.abs(dy));
        }, 0);
      });
      expect(worst).toBeLessThanOrEqual(1);
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

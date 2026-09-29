/**
 * Deliberate, human-looking pointer work for the recordings: eased moves
 * (not Playwright's straight constant-speed line), a beat before clicking,
 * and typing at a readable pace.
 */
const pos = new WeakMap();
const ease = (t) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2);

export async function moveTo(page, x, y, ms = 900) {
  const from = pos.get(page) ?? { x: x - 300, y: y + 200 };
  const steps = Math.max(2, Math.round(ms / 16));
  for (let i = 1; i <= steps; i++) {
    const k = ease(i / steps);
    // A slight arc feels less robotic than a straight line.
    const arc = Math.sin(Math.PI * (i / steps)) * Math.min(40, Math.hypot(x - from.x, y - from.y) * 0.08);
    await page.mouse.move(from.x + (x - from.x) * k, from.y + (y - from.y) * k - arc);
    await page.waitForTimeout(16);
  }
  pos.set(page, { x, y });
}

export async function moveToEl(page, locator, ms = 900, dx = 0, dy = 0) {
  await locator.waitFor({ state: "visible", timeout: 15000 });
  const b = await locator.boundingBox();
  if (!b) throw new Error("no box for " + locator);
  await moveTo(page, b.x + b.width / 2 + dx, b.y + b.height / 2 + dy, ms);
}

export async function click(page, locator, { ms = 900, pause = 250, dx = 0, dy = 0 } = {}) {
  await moveToEl(page, locator, ms, dx, dy);
  await page.waitForTimeout(pause);
  await page.mouse.down();
  await page.waitForTimeout(70);
  await page.mouse.up();
}

export async function type(page, text, delay = 38) {
  await page.keyboard.type(text, { delay });
}

export async function smoothScroll(page, dy, ms = 1400) {
  await page.evaluate(
    ({ dy, ms }) =>
      new Promise((done) => {
        const start = scrollY;
        const t0 = performance.now();
        const ease = (t) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);
        const step = (now) => {
          const k = Math.min(1, (now - t0) / ms);
          scrollTo(0, start + dy * ease(k));
          k < 1 ? requestAnimationFrame(step) : done();
        };
        requestAnimationFrame(step);
      }),
    { dy, ms },
  );
}

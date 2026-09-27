import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { startCapture } from "./lib/capture.mjs";
import { click, moveTo, moveToEl, smoothScroll, type } from "./lib/mouse.mjs";

const HERE = fileURLToPath(new URL(".", import.meta.url));
export const APP = process.env.APP_URL ?? "http://localhost:3000";
const API = process.env.API_URL ?? "http://localhost:8000";
const CURSOR = readFileSync(join(HERE, "lib/cursor.js"), "utf8");

/** Demo accounts from web/.env.test-users (created for this video). */
function account(prefix) {
  const env = Object.fromEntries(
    readFileSync(join(HERE, "../web/.env.test-users"), "utf8")
      .split("\n")
      .filter((l) => l.includes("=") && !l.startsWith("#"))
      .map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)]),
  );
  return { username: env[`${prefix}_USERNAME`], password: env[`${prefix}_PASSWORD`] };
}
export const HOST = account("DEMO_HOST");
export const GUEST = account("DEMO_GUEST");

/** Every scene starts the same: English, Movies mode, "For You" feed. */
const PREFS = {
  "movieapp-locale": { state: { locale: "en" }, version: 0 },
  "movieapp-media-mode": { state: { mode: "movie" }, version: 0 },
  "movieapp-feed-storage": { state: { isMuted: true, genres: { movie: 0, tv: 0 } }, version: 1 },
};

/**
 * Signs `who` in through the app's own dialog (so the session cookie and
 * the app's session state are both real) and returns the storage state.
 */
export async function signIn(browser, who) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(APP + "/", { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Sign in" }).first().click();
  await page.getByLabel("Username or email").fill(who.username);
  await page.getByLabel("Password", { exact: true }).fill(who.password);
  await page.locator("form button[type=submit]").click();
  await page.waitForFunction(() => JSON.parse(localStorage.getItem("movieapp-auth") || "{}").state?.username);
  const state = await ctx.storageState();
  await ctx.close();
  const origin = state.origins.find((o) => o.origin === new URL(APP).origin);
  for (const [name, value] of Object.entries(PREFS)) {
    origin.localStorage = origin.localStorage.filter((e) => e.name !== name);
    origin.localStorage.push({ name, value: JSON.stringify(value) });
  }
  return state;
}

async function scenePage(browser, state, { width = 1920, height = 1080, phone = false } = {}) {
  const ctx = await browser.newContext({
    viewport: { width, height },
    deviceScaleFactor: phone ? 2 : 1,
    ...(phone && { isMobile: true, hasTouch: true, userAgent: IPHONE_UA }),
    storageState: state,
  });
  await ctx.addInitScript(CURSOR);
  const page = await ctx.newPage();
  return { ctx, page };
}

const settle = (page, ms) => page.waitForTimeout(ms);

const IPHONE_UA =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1";
/** The phone's viewport (CSS px) and its screencast size (2x). */
export const PHONE = { width: 390, height: 844 };
export const PHONE_PX = { width: 780, height: 1688 };
export const DESKTOP = { width: 1440, height: 900 };

/** The feed post filling the screen now. */
async function currentPost(page) {
  const i = await page.evaluate(() =>
    [...document.querySelectorAll(".snap-center")].findIndex((el) => Math.abs(el.getBoundingClientRect().top) < 40),
  );
  return page.locator(".snap-center").nth(Math.max(0, i));
}

/** One swipe up the feed. (A wheel over the trailer goes to YouTube's
 * iframe, so the feed is scrolled itself; scroll-smooth animates it.) */
async function swipe(page, dwell) {
  await page.evaluate(() => document.querySelector(".snap-center")?.parentElement?.scrollBy({ top: innerHeight }));
  await settle(page, dwell);
}

/**
 * Camera moves: `zoom.in(locator, z)` pushes in on an element (anchored on
 * it, so it stays put while everything else grows), `zoom.out()` pulls back.
 * Recorded as wall-clock windows; build.mjs renders them (lib/ffmpeg zoom).
 */
function camera(page) {
  const windows = [];
  return {
    windows,
    async in(target, z = 1.5) {
      const b = typeof target.boundingBox === "function" ? await target.boundingBox() : target;
      windows.push({ a: Date.now() / 1000, x: b.x + (b.width ?? 0) / 2, y: b.y + (b.height ?? 0) / 2, z });
    },
    out() {
      const w = windows.at(-1);
      if (w && !w.b) w.b = Date.now() / 1000;
    },
  };
}

// --- 1. One tap, two universes -------------------------------------------

export async function modeSwitch(browser, state, dir) {
  const { ctx, page } = await scenePage(browser, state);
  await page.goto(APP + "/", { waitUntil: "networkidle" });
  await moveTo(page, 1180, 620, 10);
  await settle(page, 2500); // hero art and rows in
  const cap = await startCapture(page, dir);
  await settle(page, 900);
  const logo = page.locator("header button[aria-pressed]");
  const cam = camera(page);
  await moveToEl(page, logo, 1300);
  await cam.in(logo, 1.9);
  await settle(page, 900); // the ⇄ hint appears on hover
  await click(page, logo, { ms: 10, pause: 50 });
  await settle(page, 1100); // Cut flips to Show
  cam.out();
  await settle(page, 1700); // KinoShow: hero and rows crossfade to series
  await moveTo(page, 1050, 700, 900);
  await smoothScroll(page, 640, 1800);
  await settle(page, 1300);
  const rec = await cap.stop();
  await ctx.close();
  rec.zooms = cam.windows;
  return rec;
}

// --- 2. Vertical cinema discovery -----------------------------------------

export async function discover(browser, state, dir) {
  const { ctx, page } = await scenePage(browser, state);
  await page.goto(APP + "/discover", { waitUntil: "domcontentloaded" });
  await page.locator(".snap-center iframe").first().waitFor({ timeout: 20000 });
  await moveTo(page, 1300, 560, 10);
  await settle(page, 5000); // the first trailer is playing
  const cap = await startCapture(page, dir);
  await settle(page, 900);

  // Swipe through the feed: each post is a trailer, snapped full screen.
  await swipe(page, 1400);
  await swipe(page, 1400);
  await swipe(page, 1200);

  // The genre pill -> popover -> Sci-Fi.
  const pill = page.locator('button[aria-label^="Genre:"]');
  const cam = camera(page);
  await moveToEl(page, pill, 800);
  await cam.in(pill, 1.6);
  await click(page, pill, { ms: 10, pause: 50 });
  await settle(page, 600);
  await click(page, page.getByRole("option", { name: "Sci-Fi" }), { ms: 800 });
  await settle(page, 300);
  cam.out();
  await page.locator(".snap-center iframe").first().waitFor({ timeout: 20000 });
  await settle(page, 1500);
  await swipe(page, 1300);

  // Comments: the spring bottom sheet, then away.
  const post = await currentPost(page);
  await click(page, post.getByRole("button", { name: "Comments" }), { ms: 800 });
  await settle(page, 1500);
  await page.keyboard.press("Escape");
  await settle(page, 600);

  // "…" -> Details: the title opens in a modal over the still-playing feed.
  await click(page, post.getByRole("button", { name: "More actions" }), { ms: 900 });
  await settle(page, 600);
  await click(page, page.getByRole("menuitem", { name: "Details" }), { ms: 600 });
  await page.locator("[role=dialog] h1").waitFor({ timeout: 15000 });
  await settle(page, 1000);
  await moveTo(page, 960, 700, 500);
  await page.mouse.wheel(0, 500);
  await settle(page, 1100);
  const rec = await cap.stop();
  await ctx.close();
  rec.zooms = cam.windows;
  return rec;
}

// --- 3. Your language, your AI --------------------------------------------

export class RetryLater extends Error {}

export async function languageAndAI(browser, state, dir) {
  const { ctx, page } = await scenePage(browser, state);
  await page.goto(APP + "/", { waitUntil: "networkidle" });
  await moveTo(page, 1200, 560, 10);
  await settle(page, 2200);
  const cap = await startCapture(page, dir);
  await settle(page, 400);

  // TR, live: account menu -> TR.
  const cam = camera(page);
  const account = page.locator('header button[aria-label="Account menu"]');
  await moveToEl(page, account, 900);
  await cam.in(account, 1.7);
  await click(page, account, { ms: 10, pause: 50 });
  await settle(page, 350);
  await click(page, page.locator('[role=menu] [role=radio][lang="tr"]'), { ms: 600 });
  await settle(page, 1000);
  cam.out();
  await page.keyboard.press("Escape");
  await moveTo(page, 1100, 520, 500);
  await settle(page, 250);

  // The concierge, in Turkish.
  await click(page, page.locator("button.fixed.right-5"), { ms: 900 });
  const input = page.locator("aside textarea");
  await input.waitFor({ timeout: 10000 });
  await settle(page, 500);
  await click(page, input, { ms: 500 });
  await cam.in(input, 1.6);
  const answered = page.waitForResponse((r) => r.url().includes("/api/v1/chat"), { timeout: 60000 });
  await type(page, "Bu akşam için akıl yakan bir bilim kurgu öner", 30);
  await settle(page, 250);
  await page.keyboard.press("Enter");
  const sentAt = Date.now() / 1000;
  const res = await answered;
  const answeredAt = Date.now() / 1000;
  await settle(page, 700);
  cam.out();
  const body = await res.json().catch(() => ({}));
  if (!res.ok() || body.fallback) {
    await cap.stop();
    await ctx.close();
    throw new RetryLater(body.fallback ? "Groq is rate limited (keyword picks)" : `chat ${res.status()}`);
  }
  const card = page.locator('aside a[href^="/movies/"]').first();
  await card.waitFor({ timeout: 10000 });
  await settle(page, 2000); // read the answer
  await moveToEl(page, card, 800);
  await settle(page, 500);
  await click(page, card, { ms: 10, pause: 50 });
  await page.locator("[role=dialog] h1").waitFor({ timeout: 15000 });
  await settle(page, 1600);
  const rec = await cap.stop();
  await ctx.close();
  // The model's think time: keep a beat of the typing dots, drop the rest.
  if (answeredAt - sentAt > 1.8) rec.cuts = [[sentAt + 1.4, answeredAt - 0.2]];
  rec.zooms = cam.windows;
  return rec;
}

// --- 4. Watch together, in sync -------------------------------------------

export async function watchParty(browser, hostState, guestState, dirHost, dirGuest) {
  const host = await scenePage(browser, hostState, DESKTOP);
  const guest = await scenePage(browser, guestState, { ...PHONE, phone: true });
  const movie = APP + "/movies/27205"; // Inception: has a trailer

  await host.page.goto(movie, { waitUntil: "networkidle" });
  await guest.page.goto(APP + "/", { waitUntil: "networkidle" });
  const party = host.page.locator("#watch-party");
  await party.scrollIntoViewIfNeeded();
  await host.page.evaluate(() => window.scrollBy(0, -80));
  await moveTo(host.page, 900, 420, 10);
  await settle(host.page, 1500);

  const capH = await startCapture(host.page, dirHost, DESKTOP);
  const capG = await startCapture(guest.page, dirGuest, PHONE_PX);
  await settle(host.page, 400);

  // Host starts a private party; the guest opens its invite link on the phone.
  await click(host.page, party.getByRole("button", { name: "Start a private party" }), { ms: 800 });
  await party.getByText("Private", { exact: true }).waitFor({ timeout: 10000 });
  await settle(host.page, 400);
  const invite = host.page.url();
  await guest.page.goto(invite, { waitUntil: "domcontentloaded" });
  const gParty = guest.page.locator("#watch-party");
  await gParty.getByTitle("People in this room").waitFor({ timeout: 20000 });
  await guest.page.evaluate(() => {
    document.getElementById("watch-party")?.scrollIntoView({ block: "start" });
    window.scrollBy(0, -64);
  });
  // Both players ready (the trailer's length is known).
  for (const p of [host.page, guest.page]) {
    await p.waitForFunction(() => /\/ [1-9]/.test(document.getElementById("watch-party")?.innerText ?? ""), null, { timeout: 30000 });
  }
  await settle(host.page, 600);

  // Host presses play: the desktop and the phone play at the same moment.
  await click(host.page, party.locator('button[aria-label="Play"]').last(), { ms: 800 });
  await settle(host.page, 2400);

  // Chat, both ways, the camera on whoever is typing.
  const camH = camera(host.page);
  const camG = camera(guest.page);
  const hInput = party.locator('input[placeholder^="Send"]');
  await moveToEl(host.page, hInput, 700);
  await camH.in(hInput, 1.6);
  await click(host.page, hInput, { ms: 10, pause: 80 });
  await type(host.page, "Popcorn ready? 🍿", 32);
  await host.page.keyboard.press("Enter");
  await settle(host.page, 500);
  camH.out();
  const gInput = gParty.locator('input[placeholder^="Send"]');
  await camG.in(gInput, 1.6);
  await click(guest.page, gInput, { ms: 10, pause: 120 });
  await type(guest.page, "In sync! 🎬", 32);
  await guest.page.keyboard.press("Enter");
  await guest.page.evaluate(() => document.activeElement?.blur());
  await settle(host.page, 700);
  camG.out();
  await settle(host.page, 400);

  // The guest taps pause on the phone: the host pauses too.
  const pause = gParty.locator('button[aria-label="Pause"]').last();
  await pause.scrollIntoViewIfNeeded();
  await click(guest.page, pause, { ms: 10, pause: 120 });
  await settle(host.page, 1500);

  const [recH, recG] = await Promise.all([capH.stop(), capG.stop()]);
  recH.zooms = camH.windows;
  recG.zooms = camG.windows;
  await host.ctx.close();
  await guest.ctx.close();
  return { recH, recG };
}

/** The Watch Party stage stills: backdrop and device bezels. */
export async function deviceStills(browser, bgPng, framePng) {
  const ctx = await browser.newContext({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
  const page = await ctx.newPage();
  for (const [layer, file, omitBackground] of [["bg", bgPng, false], ["frame", framePng, true]]) {
    await page.goto("file://" + join(HERE, "cards/devices.html") + "?layer=" + layer, { waitUntil: "networkidle" });
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: file, omitBackground });
  }
  await ctx.close();
}

// --- Title cards -------------------------------------------------------------

export async function card(browser, dir, params, seconds) {
  const ctx = await browser.newContext({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
  const page = await ctx.newPage();
  const url = new URL("file://" + join(HERE, "cards/card.html"));
  for (const [k, v] of Object.entries({ ...params, rec: "1" })) url.searchParams.set(k, v);
  await page.goto(url.href, { waitUntil: "networkidle" });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(300);
  const cap = await startCapture(page, dir);
  await page.waitForTimeout(120);
  await page.evaluate(() => window.__go());
  await page.waitForTimeout(seconds * 1000);
  const rec = await cap.stop();
  await ctx.close();
  return rec;
}

export { API };

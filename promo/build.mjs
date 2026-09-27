/**
 * The KinoCut ⇄ KinoShow launch video, generated from the running app.
 *
 *   npm run build                 # record what's missing, assemble
 *   npm run build -- --fresh      # re-record everything
 *   npm run build -- --redo=ai,discover
 *   npm run build -- --music=track.mp3   # your own track instead of the score
 *
 * Needs the stack up (docker compose up -d), the web dev server on :3000,
 * ffmpeg on PATH, Google Chrome, and the demo accounts in
 * web/.env.test-users. Writes kinora-launch-promo.mp4 and hero-preview.gif
 * to the repository root.
 */
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

import { clipTime, crossfadeAll, devices, duration, encode, ffmpeg, FPS, gif, zoom } from "./lib/ffmpeg.mjs";
import * as S from "./scenes.mjs";

const HERE = fileURLToPath(new URL(".", import.meta.url));
const ROOT = join(HERE, "..");
const OUT = join(HERE, "out");
const SEG = join(OUT, "seg");
const FINAL = join(ROOT, "kinora-launch-promo.mp4");
const GIF = join(ROOT, "hero-preview.gif");
const FADE = 0.55;

const args = process.argv.slice(2);
const fresh = args.includes("--fresh");
const music = args.find((a) => a.startsWith("--music="))?.split("=")[1];
const redo = new Set((args.find((a) => a.startsWith("--redo="))?.split("=")[1] ?? "").split(",").filter(Boolean));
const log = (...m) => console.log(new Date().toISOString().slice(11, 19), ...m);

// The storyboard: title cards introduce each live scene. `seconds` is a
// card's length; `target` is a live clip's slot (a longer take is sped up to
// fit, at most MAX_SPEED).
const MAX_SPEED = 1.8;
const STORY = [
  { id: "intro", card: { type: "intro" }, seconds: 3.4 },
  { id: "t-mode", card: { type: "title", eyebrow: "KinoCut ⇄ KinoShow", title: "One Tap. *Two* *Universes.*", sub: "A 3D mode switch turns the whole app from films to series." }, seconds: 2.5 },
  { id: "mode", live: "modeSwitch", target: 7.2 },
  { id: "t-discover", card: { type: "title", eyebrow: "Discover", title: "Vertical *Cinema* Discovery.", sub: "Trailer feed · genre popover · spring comments · details over the feed" }, seconds: 2.5 },
  { id: "discover", live: "discover", target: 12.4 },
  { id: "t-ai", card: { type: "title", eyebrow: "TR | EN · Groq AI Concierge", title: "Your Language. *Your* *AI.*", sub: "Switch languages live. Ask in Turkish, get real, clickable picks." }, seconds: 2.5 },
  { id: "ai", live: "ai", target: 10.5 },
  { id: "t-party", card: { type: "title", eyebrow: "Real-Time Watch Party", title: "Watch Together. *In* *Sync.*", sub: "WebSocket rooms · invite links · synced playback & chat" }, seconds: 2.5 },
  { id: "party", live: "party", target: 9.5 },
  { id: "arch", card: { type: "arch" }, seconds: 7.8 },
  { id: "load", card: { type: "load" }, seconds: 5.2 },
  { id: "outro", card: { type: "outro" }, seconds: 4.0 },
];

function frames(id) {
  const dir = join(OUT, "frames", id);
  rmSync(dir, { recursive: true, force: true });
  return dir;
}

/** encode(), then the scene's camera moves (if any). */
function encodeZoomed(rec, raw) {
  encode(rec, raw);
  const wins = (rec.zooms ?? []).map((w) => ({ ...w, a: clipTime(rec, w.a), b: clipTime(rec, w.b ?? w.a + 2) }));
  if (!wins.length) return;
  zoom(raw, raw + ".z.mp4", wins);
  renameSync(raw + ".z.mp4", raw);
}

/** Speeds a clip up (never slows it down) to fit `target` seconds. */
function retime(src, out, target) {
  const k = Math.min(MAX_SPEED, Math.max(1, duration(src) / target));
  ffmpeg(["-i", src, "-vf", `setpts=PTS/${k.toFixed(4)},fps=${FPS}`, "-r", String(FPS),
    "-c:v", "libx264", "-preset", "slow", "-crf", "16", "-pix_fmt", "yuv420p", out]);
  return k;
}

async function main() {
  mkdirSync(SEG, { recursive: true });
  const need = (id) => fresh || redo.has(id) || !existsSync(join(SEG, `${id}.mp4`));
  const browser = await chromium.launch({
    channel: "chrome",
    headless: true,
    args: ["--autoplay-policy=no-user-gesture-required", "--hide-scrollbars", "--force-color-profile=srgb"],
  });

  try {
    const liveNeeded = STORY.some((s) => s.live && need(s.id));
    const host = liveNeeded ? await S.signIn(browser, S.HOST) : null;
    const guest = STORY.some((s) => s.id === "party" && need(s.id)) ? await S.signIn(browser, S.GUEST) : null;
    if (liveNeeded) log("signed in as", S.HOST.username, "and", S.GUEST.username);

    for (const step of STORY) {
      const raw = join(SEG, `${step.id}.raw.mp4`);
      const seg = join(SEG, `${step.id}.mp4`);
      if (!need(step.id)) continue;
      log("recording", step.id);
      if (step.card) {
        encode(await S.card(browser, frames(step.id), step.card, step.seconds), seg);
        continue;
      }
      if (step.live === "modeSwitch") encodeZoomed(await S.modeSwitch(browser, host, frames(step.id)), raw);
      if (step.live === "discover") encodeZoomed(await S.discover(browser, host, frames(step.id)), raw);
      if (step.live === "ai") {
        // The real model, not the keyword fallback: wait out a rate limit.
        for (let attempt = 1; ; attempt++) {
          try {
            encodeZoomed(await S.languageAndAI(browser, host, frames(step.id)), raw);
            break;
          } catch (err) {
            if (!(err instanceof S.RetryLater) || attempt === 6) throw err;
            log(`  ${err.message}; retrying in 30s (attempt ${attempt})`);
            await new Promise((r) => setTimeout(r, 30000));
          }
        }
      }
      if (step.live === "party") {
        const { recH, recG } = await S.watchParty(browser, host, guest, frames("party-host"), frames("party-guest"));
        // Same wall clock for both: cut both to the time they share.
        const from = Math.max(recH.startedAt, recG.startedAt);
        const to = Math.min(recH.endedAt, recG.endedAt);
        const h = join(SEG, "party-host.mp4");
        const g = join(SEG, "party-guest.mp4");
        encode(recH, h, { from, to, size: `${S.DESKTOP.width}:${S.DESKTOP.height}` });
        encode(recG, g, { from, to, size: `${S.PHONE_PX.width}:${S.PHONE_PX.height}` });
        const bg = join(SEG, "devices-bg.png");
        const frame = join(SEG, "devices-frame.png");
        await S.deviceStills(browser, bg, frame);
        devices(h, g, bg, frame, raw);
        // Camera on whoever types, in stage pixels (screens per cards/devices.html).
        const onStage = (rec, [sx, sy, sw, sh], vw) => (rec.zooms ?? []).map((w) => ({
          a: clipTime(rec, w.a, from), b: clipTime(rec, w.b ?? w.a + 2, from), z: w.z,
          x: sx + (w.x * sw) / vw, y: sy + (w.y * sw) / vw,
        }));
        const wins = [...onStage(recH, [120, 128, 1216, 760], S.DESKTOP.width), ...onStage(recG, [1452, 110, 380, 822], S.PHONE.width)];
        if (wins.length) {
          zoom(raw, raw + ".z.mp4", wins);
          renameSync(raw + ".z.mp4", raw);
        }
      }
      const k = retime(raw, seg, step.target);
      log(`  ${step.id}: ${duration(raw).toFixed(1)}s taken, ${k.toFixed(2)}x -> ${duration(seg).toFixed(1)}s`);
    }
  } finally {
    await browser.close();
  }

  log("assembling");
  const clips = STORY.map((s) => join(SEG, `${s.id}.mp4`));
  const silent = join(OUT, "video-silent.mp4");
  crossfadeAll(clips, silent, FADE);
  const lens = clips.map(duration);
  const at = (i) => lens.slice(0, i).reduce((a, d) => a + d - FADE, 0);

  // Sound: the generated score (music.py), every hit on a cut, or --music.
  const length = duration(silent);
  let track = music;
  if (!track) {
    const cues = STORY.map((s, i) => ({ id: s.id, start: at(i), kind: s.card ? "card" : "live" }));
    writeFileSync(join(OUT, "cues.json"), JSON.stringify({ length, cues }, null, 2));
    track = join(OUT, "score.wav");
    log("scoring");
    execFileSync("python3", [join(HERE, "music.py"), join(OUT, "cues.json"), track], { stdio: "inherit" });
  }
  ffmpeg(["-i", silent, "-i", track, "-map", "0:v", "-map", "1:a", "-c:v", "copy",
    "-af", `atrim=0:${length.toFixed(3)},afade=t=in:d=0.3,afade=t=out:st=${(length - 2.5).toFixed(3)}:d=2.5,loudnorm=I=-14:TP=-1.5:LRA=11,aresample=48000`,
    "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-shortest", FINAL]);

  // README GIF: the mode switch and the start of Discover (10s).
  const modeStart = at(STORY.findIndex((s) => s.id === "mode"));
  gif(FINAL, GIF, { start: modeStart + 0.4, length: 10, width: 960, fps: 15 });

  verify(clips);
}

/** Decodes the whole video (any error fails) and saves a contact sheet. */
function verify(clips) {
  const probe = JSON.parse(execFileSync("ffprobe", ["-v", "error", "-print_format", "json", "-show_format", "-show_streams", FINAL]).toString());
  const v = probe.streams.find((s) => s.codec_type === "video");
  const a = probe.streams.find((s) => s.codec_type === "audio");
  const errors = execFileSync("ffmpeg", ["-v", "error", "-i", FINAL, "-f", "null", "-"], { stdio: ["ignore", "pipe", "pipe"] }).toString();
  const sheet = join(OUT, "contact-sheet.jpg");
  ffmpeg(["-i", FINAL, "-vf", "fps=1/3,scale=480:-1,tile=5x4:padding=6:color=0x09090b", "-frames:v", "1", sheet]);
  log("verified", {
    file: FINAL,
    seconds: Number(probe.format.duration).toFixed(2),
    size: `${v.width}x${v.height}`,
    fps: v.r_frame_rate,
    codec: `${v.codec_name} ${v.profile} ${v.pix_fmt}`,
    megabytes: (Number(probe.format.size) / 1e6).toFixed(1),
    audio: a ? `${a.codec_name} ${a.sample_rate}Hz ${a.channels}ch` : "none",
    decodeErrors: errors.trim() || "none",
    segments: clips.length,
    gif: `${GIF} (${(Number(execFileSync("stat", ["-f", "%z", GIF]).toString()) / 1e6).toFixed(1)} MB)`,
    contactSheet: sheet,
  });
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});

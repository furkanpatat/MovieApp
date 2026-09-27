import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";

export const FPS = 30;
const X264 = ["-c:v", "libx264", "-preset", "slow", "-crf", "16", "-pix_fmt", "yuv420p", "-movflags", "+faststart"];

export function ffmpeg(args) {
  execFileSync("ffmpeg", ["-hide_banner", "-loglevel", "error", "-y", ...args], { stdio: "inherit" });
}

export function duration(file) {
  return Number(execFileSync("ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file]).toString().trim());
}

/**
 * Screencast frames -> constant-frame-rate H.264. Each frame lasts until the
 * next one's paint time; `from`/`to` (seconds on the capture's clock) trim,
 * and `cuts` ([a, b] windows on the same clock) are left out.
 */
export function encode({ frames, startedAt, endedAt, cuts = [] }, out, { from = startedAt, to = endedAt, size = "1920:1080" } = {}) {
  const overlap = (t0, t1) => cutOverlap(cuts, t0, t1);
  const kept = frames.filter((f) => f.t <= to);
  // Start from the last frame painted before `from` (what was on screen).
  let first = 0;
  for (let i = 0; i < kept.length; i++) if (kept[i].t <= from) first = i;
  const clip = kept.slice(first);
  const lines = ["ffconcat version 1.0"];
  clip.forEach((f, i) => {
    const t0 = Math.max(f.t, from);
    const t1 = i + 1 < clip.length ? clip[i + 1].t : to;
    const d = t1 - t0 - overlap(t0, t1);
    if (d > 0.0005) lines.push(`file '${f.file}'`, `duration ${d.toFixed(4)}`);
  });
  lines.push(`file '${clip.at(-1).file}'`);
  const list = out.replace(/\.mp4$/, ".ffconcat");
  writeFileSync(list, lines.join("\n"));
  ffmpeg(["-f", "concat", "-safe", "0", "-i", list, "-vf", `fps=${FPS},scale=${size}:flags=lanczos,setsar=1`, "-r", String(FPS), ...X264, out]);
}

function cutOverlap(cuts, t0, t1) {
  return cuts.reduce((sum, [a, b]) => sum + Math.max(0, Math.min(t1, b) - Math.max(t0, a)), 0);
}

/** A capture-clock time as a time in the encoded clip (after trim and cuts). */
export function clipTime({ startedAt, cuts = [] }, t, from = startedAt) {
  return t - from - cutOverlap(cuts, from, t);
}

/**
 * Camera moves on a finished clip: each window {a, b, x, y, z} (clip seconds,
 * clip pixels) eases in to zoom z anchored at (x, y) and back out. Rendered
 * at 2x first so the moving crop stays sub-pixel smooth.
 */
export function zoom(src, out, windows, { width = 1920, height = 1080, ramp = 0.45 } = {}) {
  const S = 2;
  const ease = windows.map(({ a, b }) => {
    const r = `min(clip((it-${a.toFixed(3)})/${ramp},0,1),clip((${b.toFixed(3)}-it)/${ramp},0,1))`;
    return `(${r})*(${r})*(3-2*(${r}))`;
  });
  const z = "1" + windows.map((w, i) => `+${(w.z - 1).toFixed(3)}*${ease[i]}`).join("");
  const anchor = (key) => windows.map((w, i) => `${(w[key] * S).toFixed(1)}*gt(${ease[i]},0)`).join("+") || "0";
  ffmpeg(["-i", src, "-vf",
    `scale=${width * S}:${height * S}:flags=lanczos,` +
    `zoompan=z='${z}':x='(${anchor("x")})*(1-1/zoom)':y='(${anchor("y")})*(1-1/zoom)':d=1:s=${width}x${height}:fps=${FPS}`,
    "-r", String(FPS), ...X264, out]);
}

/** Where the recordings sit in cards/devices.html (x, y, w, h). */
const SCREENS = { desktop: [120, 128, 1216, 760], phone: [1452, 110, 380, 822] };

/** The host on a desktop monitor, the guest on a phone: bg, both screens, bezels. */
export function devices(desktop, phone, bg, frame, out) {
  const [dx, dy, dw, dh] = SCREENS.desktop;
  const [px, py, pw, ph] = SCREENS.phone;
  ffmpeg(["-loop", "1", "-i", bg, "-i", desktop, "-i", phone, "-loop", "1", "-i", frame, "-filter_complex",
    `[1:v]scale=${dw}:${dh}:flags=lanczos[d];[2:v]scale=${pw}:${ph}:flags=lanczos[p];` +
    `[0:v][d]overlay=${dx}:${dy}:shortest=1[a];[a][p]overlay=${px}:${py}:shortest=1[b];[b][3:v]overlay=0:0:shortest=1,fps=${FPS},format=yuv420p`,
    "-r", String(FPS), ...X264, out]);
}

/** Chains clips with crossfades (xfade), `fade` seconds each. */
export function crossfadeAll(clips, out, fade = 0.5) {
  const inputs = clips.flatMap((c) => ["-i", c]);
  let filter = "";
  let prev = "[0:v]";
  let offset = 0;
  clips.slice(1).forEach((c, i) => {
    offset += duration(clips[i]) - fade;
    const label = i === clips.length - 2 ? "[v]" : `[x${i}]`;
    filter += `${prev}[${i + 1}:v]xfade=transition=fade:duration=${fade}:offset=${offset.toFixed(3)}${label};`;
    prev = label;
  });
  // Screencast JPEGs are full range: deliver standard limited-range yuv420p.
  filter = filter.replace(/\[v\];$/, "[x];[x]scale=in_range=pc:out_range=tv,format=yuv420p[v];");
  ffmpeg([...inputs, "-filter_complex", filter.replace(/;$/, ""), "-map", "[v]", "-r", String(FPS), ...X264,
    "-color_range", "tv", "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709", out]);
}

/** A short, light GIF (palette per clip) for the README. */
export function gif(src, out, { start = 0, length = 10, width = 960, fps = 15 } = {}) {
  ffmpeg(["-ss", String(start), "-t", String(length), "-i", src, "-vf",
    `fps=${fps},scale=${width}:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle`,
    "-loop", "0", out]);
}

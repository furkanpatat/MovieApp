import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/**
 * Records a page through Chrome's screencast (CDP): full-resolution JPEG
 * frames with their paint timestamps. Chrome only sends a frame when the
 * page repaints, so the result is variable-frame-rate; encode() turns it
 * into constant 30 fps, holding each frame until the next one.
 */
export async function startCapture(page, dir, { width = 1920, height = 1080 } = {}) {
  mkdirSync(dir, { recursive: true });
  const cdp = await page.context().newCDPSession(page);
  const frames = [];
  let n = 0;
  cdp.on("Page.screencastFrame", async (f) => {
    const file = join(dir, `${String(n++).padStart(5, "0")}.jpg`);
    writeFileSync(file, Buffer.from(f.data, "base64"));
    frames.push({ file, t: f.metadata.timestamp });
    await cdp.send("Page.screencastFrameAck", { sessionId: f.sessionId }).catch(() => {});
  });
  await cdp.send("Page.startScreencast", { format: "jpeg", quality: 93, maxWidth: width, maxHeight: height, everyNthFrame: 1 });
  const startedAt = Date.now() / 1000;
  return {
    startedAt,
    async stop() {
      const endedAt = Date.now() / 1000;
      await cdp.send("Page.stopScreencast").catch(() => {});
      await cdp.detach().catch(() => {});
      return { frames, startedAt, endedAt };
    },
  };
}

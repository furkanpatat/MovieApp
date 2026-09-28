import { useEffect, useMemo, useRef, useState } from "react";
import { Platform, StyleSheet } from "react-native";
import { WebView, type WebViewMessageEvent } from "react-native-webview";

/** YouTube player states (IFrame Player API). */
const PLAYING = 1;

/**
 * A YouTube trailer, full-bleed ("cover" for a 16:9 video on a tall screen),
 * through the IFrame Player API inside a WebView. Most film trailers are
 * scope (2.39:1) letterboxed in 16:9; the 1.36x overscan crops those bars
 * (each is 12.8% of the height: s >= 1 / (1 - 2 * 0.128)).
 *
 * Unlike a browser, the app decides the autoplay policy
 * (mediaPlaybackRequiresUserAction={false}), so trailers start with sound
 * as you swipe. The page is served with the site as its base URL: YouTube
 * refuses embeds that come with no referrer.
 */
export function TrailerPlayer({
  videoKey,
  playing,
  muted,
  onPlaying,
  onError,
}: {
  videoKey: string;
  playing: boolean;
  muted: boolean;
  /** The video is actually playing (time to fade the poster out). */
  onPlaying?: () => void;
  /** YouTube can't play it here (removed, private, embedding off...). */
  onError?: () => void;
}) {
  const ref = useRef<WebView>(null);
  // The first render's wishes are baked into the page; later ones are sent.
  const [initial] = useState({ playing, muted });
  const html = useMemo(() => page(videoKey, initial.muted), [videoKey, initial.muted]);

  useEffect(() => {
    ref.current?.injectJavaScript(`window.setWant && window.setWant(${playing}, ${muted}); true;`);
  }, [playing, muted]);

  const onMessage = (e: WebViewMessageEvent) => {
    try {
      const msg = JSON.parse(e.nativeEvent.data) as { state?: number; error?: number };
      if (msg.state === PLAYING) onPlaying?.();
      if (msg.error !== undefined) onError?.();
    } catch {
      // Not ours.
    }
  };

  return (
    <WebView
      ref={ref}
      source={{ html, baseUrl: "https://kinora.duckdns.org" }}
      originWhitelist={["*"]}
      style={styles.web}
      containerStyle={StyleSheet.absoluteFill}
      pointerEvents="none"
      scrollEnabled={false}
      // Full-bleed: no safe-area inset inside the page (the status bar sits over it).
      automaticallyAdjustContentInsets={false}
      contentInsetAdjustmentBehavior="never"
      bounces={false}
      // Android's WebView says it's a phone browser, and YouTube then serves
      // its mobile player, which ignores controls: 0; a desktop user agent
      // gets the chrome-free one. (iOS already gets it.)
      userAgent={Platform.OS === "android" ? DESKTOP_UA : undefined}
      allowsInlineMediaPlayback
      mediaPlaybackRequiresUserAction={false}
      allowsFullscreenVideo={false}
      javaScriptEnabled
      onMessage={onMessage}
      onLoadEnd={() => ref.current?.injectJavaScript(`window.setWant && window.setWant(${initial.playing}, ${initial.muted}); true;`)}
    />
  );
}

const DESKTOP_UA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36";

function page(key: string, muted: boolean): string {
  const id = JSON.stringify(key);
  return `<!doctype html><html><head>
<meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1,viewport-fit=cover">
<style>
  html, body { margin: 0; height: 100%; overflow: hidden; background: #000; }
  #p { position: absolute; top: 50%; left: 50%; width: max(100vw, 177.78vh); height: max(100vh, 56.25vw);
       transform: translate(-50%, -50%) scale(1.36); border: 0; pointer-events: none; }
</style></head><body><div id="p"></div>
<script>
  var player, ready = false, want = { playing: true, muted: ${muted} };
  function send(m) { window.ReactNativeWebView && window.ReactNativeWebView.postMessage(JSON.stringify(m)); }
  function apply() {
    if (!ready) return;
    want.muted ? player.mute() : player.unMute();
    want.playing ? player.playVideo() : player.pauseVideo();
  }
  window.setWant = function (playing, muted) { want.playing = playing; want.muted = muted; apply(); };
  function onYouTubeIframeAPIReady() {
    player = new YT.Player("p", {
      videoId: ${id},
      playerVars: { autoplay: 1, mute: ${muted ? 1 : 0}, controls: 0, disablekb: 1, fs: 0, iv_load_policy: 3,
                    modestbranding: 1, rel: 0, playsinline: 1, loop: 1, playlist: ${id} },
      events: {
        onReady: function () { ready = true; apply(); },
        onStateChange: function (e) { send({ state: e.data }); },
        onError: function (e) { send({ error: e.data }); }
      }
    });
  }
</script>
<script src="https://www.youtube.com/iframe_api"></script>
</body></html>`;
}

const styles = StyleSheet.create({
  web: { flex: 1, backgroundColor: "transparent" },
});

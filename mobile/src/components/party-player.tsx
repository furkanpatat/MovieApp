import { useEffect, useMemo, useRef, useState } from "react";
import { Platform, Pressable, StyleSheet, Text, View } from "react-native";
import { WebView, type WebViewMessageEvent } from "react-native-webview";

import { useT } from "@/i18n";
import { SITE } from "@/lib/party";
import { colors } from "@/theme";
import type { PlaybackAction, PlaybackState } from "@/types/watch-party";

function clock(seconds: number) {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/**
 * The Watch Party's trailer: a YouTube player (IFrame API in a WebView)
 * that follows the room. A room event is "the video was at T when this was
 * sent": playing, the target is T plus the time since; paused, it's T.
 * Tapping plays or pauses here and tells the room where (onLocal).
 */
export function PartyPlayer({
  videoKey,
  playback,
  onLocal,
}: {
  videoKey: string;
  playback: PlaybackState | null;
  onLocal: (action: PlaybackAction, position: number) => void;
}) {
  const ref = useRef<WebView>(null);
  const { t } = useT();
  const [ready, setReady] = useState(false);
  const [tick, setTick] = useState({ t: 0, d: 0, playing: false });
  const html = useMemo(() => page(videoKey), [videoKey]);

  // Follow the room: now, or as soon as the player is ready.
  useEffect(() => {
    if (!ready || !playback) return;
    const at = Date.parse(playback.updated_at);
    ref.current?.injectJavaScript(
      `window.follow(${JSON.stringify(playback.action)}, ${playback.timestamp}, ${Number.isFinite(at) ? at : Date.now()}); true;`,
    );
  }, [ready, playback]);

  const onMessage = (e: WebViewMessageEvent) => {
    let msg: { ready?: boolean; tick?: typeof tick; local?: PlaybackAction; t?: number };
    try {
      msg = JSON.parse(e.nativeEvent.data);
    } catch {
      return;
    }
    if (msg.ready) setReady(true);
    if (msg.tick) setTick(msg.tick);
    if (msg.local && typeof msg.t === "number") onLocal(msg.local, msg.t);
  };

  const progress = tick.d > 0 ? Math.min(1, tick.t / tick.d) : 0;

  return (
    <View>
      <View style={styles.frame}>
        <WebView
          ref={ref}
          source={{ html, baseUrl: SITE }}
          originWhitelist={["*"]}
          style={styles.web}
          pointerEvents="none"
          scrollEnabled={false}
          bounces={false}
          automaticallyAdjustContentInsets={false}
          contentInsetAdjustmentBehavior="never"
          // Android's WebView says it's a phone browser, and YouTube then serves
      // its mobile player, which ignores controls: 0; a desktop user agent
      // gets the chrome-free one. (iOS already gets it.)
      userAgent={Platform.OS === "android" ? DESKTOP_UA : undefined}
      allowsInlineMediaPlayback
          mediaPlaybackRequiresUserAction={false}
          allowsFullscreenVideo={false}
          onMessage={onMessage}
        />
        <Pressable
          style={StyleSheet.absoluteFill}
          onPress={() => ref.current?.injectJavaScript("window.toggle(); true;")}
          accessibilityRole="button"
          accessibilityLabel={tick.playing ? t.party.pauseAll : t.party.playAll}
        >
          {!tick.playing && ready && (
            <View style={styles.playBadge}>
              <Text style={styles.playGlyph}>▶</Text>
            </View>
          )}
        </Pressable>
        {!ready && (
          <View style={[StyleSheet.absoluteFill, styles.loading]}>
            <Text style={styles.loadingText}>{t.party.loadingTrailer}</Text>
          </View>
        )}
      </View>
      <View style={styles.bar}>
        <View style={[styles.fill, { width: `${progress * 100}%` }]} />
      </View>
      <Text style={styles.time}>
        {clock(tick.t)} / {clock(tick.d)}
      </Text>
    </View>
  );
}

const DESKTOP_UA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36";

function page(key: string): string {
  const id = JSON.stringify(key);
  return `<!doctype html><html><head>
<meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1,viewport-fit=cover">
<style>html,body{margin:0;height:100%;overflow:hidden;background:#000}#p{position:absolute;inset:0;width:100%;height:100%;border:0}</style>
</head><body><div id="p"></div>
<script>
  var player, playing = false;
  function send(m) { window.ReactNativeWebView && window.ReactNativeWebView.postMessage(JSON.stringify(m)); }
  window.follow = function (action, ts, atMs) {
    if (!player) return;
    var target = action === "pause" ? ts : ts + Math.max(0, (Date.now() - atMs) / 1000);
    player.seekTo(target, true);
    action === "pause" ? player.pauseVideo() : player.playVideo();
  };
  window.toggle = function () {
    if (!player) return;
    var t = player.getCurrentTime();
    if (playing) { player.pauseVideo(); send({ local: "pause", t: t }); }
    else { player.playVideo(); send({ local: "play", t: t }); }
  };
  function onYouTubeIframeAPIReady() {
    player = new YT.Player("p", {
      videoId: ${id},
      playerVars: { controls: 0, disablekb: 1, fs: 0, rel: 0, playsinline: 1, modestbranding: 1, iv_load_policy: 3 },
      events: {
        onReady: function () {
          send({ ready: true });
          setInterval(function () { send({ tick: { t: player.getCurrentTime(), d: player.getDuration(), playing: playing } }); }, 500);
        },
        onStateChange: function (e) { playing = e.data === 1 || e.data === 3; }
      }
    });
  }
</script>
<script src="https://www.youtube.com/iframe_api"></script>
</body></html>`;
}

const styles = StyleSheet.create({
  frame: { aspectRatio: 16 / 9, width: "100%", backgroundColor: "#000", borderRadius: 14, overflow: "hidden" },
  web: { flex: 1, backgroundColor: "#000" },
  playBadge: { position: "absolute", top: "50%", left: "50%", marginLeft: -32, marginTop: -32, width: 64, height: 64, borderRadius: 32, backgroundColor: "rgba(0,0,0,0.55)", alignItems: "center", justifyContent: "center" },
  playGlyph: { color: colors.text, fontSize: 26, marginLeft: 4 },
  loading: { alignItems: "center", justifyContent: "center" },
  loadingText: { color: colors.mute },
  bar: { height: 3, backgroundColor: colors.cardHi, borderRadius: 2, marginTop: 10, overflow: "hidden" },
  fill: { height: 3, backgroundColor: colors.gold },
  time: { color: colors.dim, fontSize: 12, marginTop: 6, fontVariant: ["tabular-nums"] },
});

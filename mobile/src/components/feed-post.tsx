import * as Haptics from "expo-haptics";
import { Image } from "expo-image";
import { LinearGradient } from "expo-linear-gradient";
import { router } from "expo-router";
import { SymbolView, type SFSymbol } from "expo-symbols";
import { memo, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { openTitle } from "@/components/poster-card";
import { TrailerPlayer } from "@/components/trailer-player";
import { useT } from "@/i18n";
import { useLike, useTitle } from "@/lib/queries";
import { backdropUrl, posterUrl } from "@/lib/tmdb";
import { useAuth } from "@/store/auth";
import { useFeed } from "@/store/feed";
import { colors } from "@/theme";
import type { MediaType, Movie } from "@/types/movie";

/**
 * One full-screen Discover post: the backdrop, then the trailer once it's
 * playing (only the post on screen plays; neighbours load their details so
 * the next swipe starts fast). Tap to pause, like, sound, details.
 */
export const FeedPost = memo(function FeedPost({
  movie,
  mode,
  height,
  active,
  near,
  liked,
}: {
  movie: Movie;
  mode: MediaType;
  height: number;
  active: boolean;
  near: boolean;
  liked: boolean;
}) {
  const insets = useSafeAreaInsets();
  const { t } = useT();
  const media = movie.media_type ?? mode;
  const details = useTitle(media, movie.id, near);
  const { muted, toggleMuted } = useFeed();
  const signedIn = useAuth((s) => !!s.token);
  const like = useLike();
  const [paused, setPaused] = useState(false);
  const [started, setStarted] = useState(false);
  const [failed, setFailed] = useState(false);

  const trailer = details.data?.trailer_key;
  const still = backdropUrl(movie.backdrop_path, "w1280") ?? posterUrl(movie.poster_path, "w780");
  const isLiked = liked || like.isPending || like.isSuccess;
  const year = movie.release_date?.slice(0, 4);

  // Leaving the screen resets the post: back at its still, playing on return.
  const [wasActive, setWasActive] = useState(active);
  if (wasActive !== active) {
    setWasActive(active);
    if (!active) {
      setPaused(false);
      setStarted(false);
    }
  }

  const onLike = () => {
    if (!signedIn) return router.push("/login");
    if (isLiked) return;
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    like.mutate({ ...movie, media_type: media });
  };

  return (
    <View style={{ height, backgroundColor: colors.bg }}>
      {still && <Image source={{ uri: still }} style={[StyleSheet.absoluteFill, started && styles.hidden]} contentFit="cover" transition={200} />}
      {active && trailer && !failed && (
        <View style={[StyleSheet.absoluteFill, !started && styles.hidden]}>
          <TrailerPlayer
            videoKey={trailer}
            playing={!paused}
            muted={muted}
            onPlaying={() => setStarted(true)}
            onError={() => (setFailed(true), setStarted(false))}
          />
        </View>
      )}

      <Pressable style={StyleSheet.absoluteFill} onPress={() => setPaused((p) => !p)} accessibilityLabel={paused ? t.discover.play : t.discover.pause}>
        {paused && (
          <View style={styles.pauseBadge}>
            <Glyph name="play.fill" fallback="▶" size={40} />
          </View>
        )}
      </Pressable>

      <LinearGradient
        colors={["rgba(9,9,11,0.55)", "transparent", "transparent", "rgba(9,9,11,0.92)"]}
        locations={[0, 0.2, 0.55, 1]}
        style={StyleSheet.absoluteFill}
        pointerEvents="none"
      />

      <View style={[styles.info, { bottom: insets.bottom + 78 }]} pointerEvents="box-none">
        <Text style={styles.title} numberOfLines={2}>
          {movie.title}
        </Text>
        <Text style={styles.meta}>
          ★ {movie.vote_average.toFixed(1)}
          {year ? `  ·  ${year}` : ""}
          {media === "tv" ? `  ·  ${t.common.series}` : ""}
        </Text>
        <Text style={styles.overview} numberOfLines={3}>
          {details.data?.overview || movie.overview}
        </Text>
      </View>

      <View style={[styles.rail, { bottom: insets.bottom + 86 }]}>
        <RailButton label={isLiked ? t.discover.liked : t.discover.like} onPress={onLike}>
          <Glyph name={isLiked ? "heart.fill" : "heart"} fallback="♥" tint={isLiked ? "#f43f5e" : colors.text} />
        </RailButton>
        <RailButton label={muted ? t.discover.soundOff : t.discover.soundOn} onPress={toggleMuted}>
          <Glyph name={muted ? "speaker.slash.fill" : "speaker.wave.2.fill"} fallback={muted ? "🔇" : "🔊"} />
        </RailButton>
        <RailButton label={t.discover.details} onPress={() => openTitle(movie, media)}>
          <Glyph name="info.circle" fallback="i" />
        </RailButton>
      </View>
    </View>
  );
});

function RailButton({ label, onPress, children }: { label: string; onPress: () => void; children: React.ReactNode }) {
  return (
    <Pressable onPress={onPress} accessibilityRole="button" accessibilityLabel={label} hitSlop={8} style={({ pressed }) => [styles.railButton, pressed && { transform: [{ scale: 0.9 }] }]}>
      {children}
    </Pressable>
  );
}

function Glyph({ name, fallback, tint = colors.text, size = 30 }: { name: SFSymbol; fallback: string; tint?: string; size?: number }) {
  return <SymbolView name={name} tintColor={tint} size={size} fallback={<Text style={{ color: tint, fontSize: size * 0.8 }}>{fallback}</Text>} />;
}

const styles = StyleSheet.create({
  hidden: { opacity: 0 },
  pauseBadge: { position: "absolute", top: "45%", alignSelf: "center", width: 84, height: 84, borderRadius: 42, backgroundColor: "rgba(0,0,0,0.45)", alignItems: "center", justifyContent: "center" },
  info: { position: "absolute", left: 16, right: 84 },
  title: { color: colors.text, fontSize: 26, fontWeight: "800", letterSpacing: -0.5, textShadowColor: "rgba(0,0,0,0.6)", textShadowRadius: 8 },
  meta: { color: colors.gold, fontWeight: "700", marginTop: 6 },
  overview: { color: "#d4d4d8", marginTop: 6, lineHeight: 20 },
  rail: { position: "absolute", right: 12, gap: 18, alignItems: "center" },
  railButton: { width: 52, height: 52, borderRadius: 26, backgroundColor: "rgba(0,0,0,0.35)", alignItems: "center", justifyContent: "center" },
});

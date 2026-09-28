import { Image } from "expo-image";
import { LinearGradient } from "expo-linear-gradient";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { LogoSwitch } from "@/components/logo-switch";
import { openTitle } from "@/components/poster-card";
import { TitleRow } from "@/components/title-row";
import { useT } from "@/i18n";
import { useLayout } from "@/lib/layout";
import { useGenre, usePopular } from "@/lib/queries";
import { backdropUrl } from "@/lib/tmdb";
import { useMode } from "@/store/mode";
import { colors } from "@/theme";
import type { MediaType } from "@/types/movie";

// TMDB genre ids differ between movies and TV.
const ROWS: Record<MediaType, number[]> = {
  movie: [28, 878, 35, 27],
  tv: [10759, 10765, 35, 80],
};

function GenreRow({ mode, title, genre }: { mode: MediaType; title: string; genre: number }) {
  const q = useGenre(mode, genre);
  return <TitleRow title={title} items={q.data?.results} mode={mode} loading={q.isPending} />;
}

export default function Home() {
  const mode = useMode((s) => s.mode);
  const { t } = useT();
  const popular = usePopular(mode);
  const insets = useSafeAreaInsets();
  const layout = useLayout();
  const hero = popular.data?.results.find((m) => m.backdrop_path);
  const heroHeight = layout.hero;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={{ paddingBottom: 120 }} contentInsetAdjustmentBehavior="never">
      <View style={{ height: heroHeight }}>
        {hero && (
          <Image source={{ uri: backdropUrl(hero.backdrop_path, "w1280")! }} style={StyleSheet.absoluteFill} contentFit="cover" transition={300} />
        )}
        <LinearGradient colors={["rgba(9,9,11,0.7)", "transparent", "rgba(9,9,11,0.2)", colors.bg]} locations={[0, 0.25, 0.6, 1]} style={StyleSheet.absoluteFill} />
        <View style={[styles.top, { paddingTop: insets.top + 8, left: layout.gutter }]}>
          <LogoSwitch />
        </View>
        {hero && (
          <View style={[styles.heroText, { left: layout.gutter, maxWidth: Math.min(layout.width - layout.gutter * 2, 640) }]}>
            <Text style={styles.heroTitle} numberOfLines={2}>
              {hero.title}
            </Text>
            <Text style={styles.heroMeta}>
              ★ {hero.vote_average.toFixed(1)} · {hero.release_date?.slice(0, 4)}
            </Text>
            <Text style={styles.heroOverview} numberOfLines={3}>
              {hero.overview}
            </Text>
            <Pressable onPress={() => openTitle(hero, mode)} style={({ pressed }) => [styles.cta, pressed && { opacity: 0.8 }]}>
              <Text style={styles.ctaText}>{t.home.moreInfo}</Text>
            </Pressable>
          </View>
        )}
      </View>

      <TitleRow title={mode === "tv" ? t.home.trendingSeries : t.home.trendingMovies} items={popular.data?.results} mode={mode} loading={popular.isPending} />
      {ROWS[mode].map((genre) => (
        <GenreRow key={`${mode}-${genre}`} mode={mode} title={t.home.genres[genre]} genre={genre} />
      ))}
      {popular.isError && <Text style={styles.error}>{t.home.offline}</Text>}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  top: { position: "absolute", left: 16, right: 16 },
  heroText: { position: "absolute", bottom: 12 },
  heroTitle: { color: colors.text, fontSize: 34, fontWeight: "800", letterSpacing: -0.8 },
  heroMeta: { color: colors.gold, fontWeight: "700", marginTop: 6 },
  heroOverview: { color: colors.mute, marginTop: 8, lineHeight: 20 },
  cta: { alignSelf: "flex-start", marginTop: 14, backgroundColor: colors.gold, paddingHorizontal: 18, paddingVertical: 10, borderRadius: 999 },
  ctaText: { color: colors.onGold, fontWeight: "800" },
  error: { color: colors.danger, textAlign: "center", marginTop: 24 },
});

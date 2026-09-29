import { Image } from "expo-image";
import { LinearGradient } from "expo-linear-gradient";
import * as Haptics from "expo-haptics";
import { router, useLocalSearchParams } from "expo-router";
import * as WebBrowser from "expo-web-browser";
import { useMemo } from "react";
import { ActivityIndicator, FlatList, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import { useT } from "@/i18n";
import { useLayout } from "@/lib/layout";
import { newPartyCode } from "@/lib/party";
import { useLibrary, useTitle, useToggleLibrary } from "@/lib/queries";
import { backdropUrl, posterUrl, profileUrl } from "@/lib/tmdb";
import { useAuth } from "@/store/auth";
import { colors, radius } from "@/theme";
import type { MediaType, Movie } from "@/types/movie";

interface CastMember {
  id: number;
  name: string;
  character?: string;
  profile_path?: string | null;
}

/** cast_json is TMDB's credits.cast, stored verbatim by the catalog. */
function parseCast(json?: string): CastMember[] {
  if (!json) return [];
  try {
    const raw: unknown = JSON.parse(json);
    return Array.isArray(raw) ? (raw as CastMember[]).filter((c) => c && c.name).slice(0, 15) : [];
  } catch {
    return [];
  }
}

/** My List and Watched for one title, synced with the web library. */
function LibraryButtons({ movie, media }: { movie: Movie; media: MediaType }) {
  const { t } = useT();
  const signedIn = useAuth((s) => !!s.token);
  const { list, watched } = useLibrary();
  const toggleList = useToggleLibrary("watchlist");
  const toggleWatched = useToggleLibrary("watched");
  const same = (m: Movie) => m.id === movie.id && (m.media_type ?? "movie") === media;
  const inList = !!list.data?.items.some((i) => same(i.movie as Movie));
  const isWatched = !!watched.data?.items.some((i) => same(i.movie as Movie));
  const saved = toggleList.isPending ? toggleList.variables?.on : inList;
  const seen = toggleWatched.isPending ? toggleWatched.variables?.on : isWatched;

  const press = (toggle: typeof toggleList, on: boolean) => {
    if (!signedIn) return router.push("/login");
    void Haptics.selectionAsync();
    toggle.mutate({ movie, media, on });
  };

  return (
    <View style={styles.libRow}>
      <Pressable onPress={() => press(toggleList, !saved)} style={({ pressed }) => [styles.libButton, saved && styles.libOn, pressed && { opacity: 0.8 }]}>
        <Text style={[styles.libText, saved && styles.libTextOn]}>{saved ? t.title.inList : t.title.addToList}</Text>
      </Pressable>
      <Pressable onPress={() => press(toggleWatched, !seen)} style={({ pressed }) => [styles.libButton, seen && styles.libOn, pressed && { opacity: 0.8 }]}>
        <Text style={[styles.libText, seen && styles.libTextOn]}>{seen ? t.title.watched : t.title.markWatched}</Text>
      </Pressable>
    </View>
  );
}

/** A movie or series: backdrop, poster, facts, trailer, library and cast. */
export default function TitleScreen() {
  const params = useLocalSearchParams<{ media: string; id: string }>();
  const media: MediaType = params.media === "tv" ? "tv" : "movie";
  const { t: tr } = useT();
  const q = useTitle(media, Number(params.id));
  const layout = useLayout();
  const t = q.data;
  const cast = useMemo(() => parseCast(t?.cast_json), [t?.cast_json]);

  if (q.isPending) return <ActivityIndicator style={styles.center} color={colors.gold} />;
  if (q.isError || !t) return <Text style={[styles.center, styles.error]}>{tr.title.failed}</Text>;

  const backdrop = backdropUrl(t.backdrop_path, "w1280");
  const poster = posterUrl(t.poster_path, "w342");
  const year = t.release_date?.slice(0, 4);
  const facts = [
    year,
    media === "movie" && t.runtime ? `${Math.floor(t.runtime / 60)}h ${t.runtime % 60}m` : null,
    media === "tv" && t.number_of_seasons ? tr.title.seasons(t.number_of_seasons) : null,
    t.rated,
  ].filter(Boolean);

  return (
    <ScrollView style={styles.screen} contentContainerStyle={{ paddingBottom: 60 }}>
      <View style={{ height: Math.min(layout.width * 0.75, layout.height * 0.55) }}>
        {backdrop && <Image source={{ uri: backdrop }} style={StyleSheet.absoluteFill} contentFit="cover" transition={250} />}
        <LinearGradient colors={["rgba(9,9,11,0.55)", "transparent", colors.bg]} locations={[0, 0.35, 1]} style={StyleSheet.absoluteFill} />
      </View>

      {/* The details read in a centered column on wide screens. */}
      <View style={{ width: "100%", maxWidth: layout.readable, alignSelf: "center" }}>
        <View style={styles.headRow}>
          {poster && <Image source={{ uri: poster }} style={styles.poster} contentFit="cover" />}
          <View style={{ flex: 1 }}>
            {media === "tv" && <Text style={styles.badge}>{tr.title.series}</Text>}
            <Text style={styles.title}>{t.title}</Text>
            <Text style={styles.facts}>{facts.join(" · ")}</Text>
            <View style={styles.ratings}>
              {t.imdb_rating ? <Text style={styles.imdb}>IMDb {t.imdb_rating.toFixed(1)}</Text> : null}
              <Text style={styles.tmdb}>★ {t.vote_average.toFixed(1)}</Text>
            </View>
          </View>
        </View>

        {t.trailer_key && (
          <Pressable
            onPress={() => void WebBrowser.openBrowserAsync(`https://www.youtube.com/watch?v=${t.trailer_key}`)}
            style={({ pressed }) => [styles.trailer, pressed && { opacity: 0.8 }]}
          >
            <Text style={styles.trailerText}>▶  {tr.title.playTrailer}</Text>
          </Pressable>
        )}

        <LibraryButtons movie={t} media={media} />

        {media === "movie" && t.trailer_key && (
          <View style={styles.party}>
            <Text style={styles.partyTitle}>{tr.title.watchTogether}</Text>
            <Text style={styles.partySub}>{tr.title.watchTogetherSub}</Text>
            <View style={[styles.libRow, styles.inCard]}>
              <Pressable
                onPress={() => router.push({ pathname: "/party/[id]", params: { id: String(t.id), code: newPartyCode() } })}
                style={({ pressed }) => [styles.libButton, styles.libOn, pressed && { opacity: 0.8 }]}
              >
                <Text style={[styles.libText, styles.libTextOn]}>{tr.title.privateParty}</Text>
              </Pressable>
              <Pressable
                onPress={() => router.push({ pathname: "/party/[id]", params: { id: String(t.id) } })}
                style={({ pressed }) => [styles.libButton, pressed && { opacity: 0.8 }]}
              >
                <Text style={styles.libText}>{tr.title.openRoom}</Text>
              </Pressable>
            </View>
          </View>
        )}

        {t.genres && t.genres.length > 0 && (
          <View style={styles.chips}>
            {t.genres.map((g) => (
              <Text key={g.id} style={styles.chip}>
                {g.name}
              </Text>
            ))}
          </View>
        )}

        {t.tagline ? <Text style={styles.tagline}>“{t.tagline}”</Text> : null}
        <Text style={styles.overview}>{t.overview}</Text>

        {cast.length > 0 && (
          <>
            <Text style={styles.section}>{tr.title.topCast}</Text>
            <FlatList
              horizontal
              data={cast}
              keyExtractor={(c) => String(c.id)}
              showsHorizontalScrollIndicator={false}
              contentContainerStyle={{ paddingHorizontal: 16, gap: 12 }}
              renderItem={({ item }) => {
                const uri = profileUrl(item.profile_path);
                return (
                  <View style={styles.castItem}>
                    <View style={styles.castPhoto}>
                      {uri ? <Image source={{ uri }} style={StyleSheet.absoluteFill} contentFit="cover" /> : <Text style={styles.castInitial}>{item.name[0]}</Text>}
                    </View>
                    <Text style={styles.castName} numberOfLines={2}>
                      {item.name}
                    </Text>
                    {item.character ? (
                      <Text style={styles.castRole} numberOfLines={1}>
                        {item.character}
                      </Text>
                    ) : null}
                  </View>
                );
              }}
            />
          </>
        )}

        {(t.director || t.creators?.length) && (
          <Text style={styles.credit}>
            {media === "tv" ? tr.title.createdBy : tr.title.directedBy}
            <Text style={{ color: colors.text }}>{t.director ?? t.creators?.join(", ")}</Text>
          </Text>
        )}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  center: { flex: 1, marginTop: 120, alignSelf: "center" },
  error: { color: colors.danger },
  headRow: { flexDirection: "row", gap: 14, paddingHorizontal: 16, marginTop: -90 },
  poster: { width: 110, height: 165, borderRadius: radius.sm, borderWidth: 1, borderColor: colors.border },
  badge: { color: colors.gold, fontSize: 11, fontWeight: "800", letterSpacing: 1.5, marginTop: 40 },
  title: { color: colors.text, fontSize: 26, fontWeight: "800", letterSpacing: -0.5, marginTop: 4 },
  facts: { color: colors.mute, marginTop: 6 },
  ratings: { flexDirection: "row", gap: 10, marginTop: 8, alignItems: "center" },
  imdb: { backgroundColor: colors.gold, color: colors.onGold, fontWeight: "900", paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4, overflow: "hidden", fontSize: 12 },
  tmdb: { color: colors.text, fontWeight: "700" },
  trailer: { marginHorizontal: 16, marginTop: 18, backgroundColor: colors.gold, borderRadius: 999, paddingVertical: 13, alignItems: "center" },
  trailerText: { color: colors.onGold, fontWeight: "800", fontSize: 16 },
  party: { marginHorizontal: 16, marginTop: 20, padding: 16, borderRadius: radius.md, backgroundColor: colors.card, borderWidth: 1, borderColor: colors.border },
  partyTitle: { color: colors.text, fontSize: 17, fontWeight: "800" },
  partySub: { color: colors.mute, marginTop: 4, marginBottom: 2 },
  inCard: { marginHorizontal: 0 },
  libRow: { flexDirection: "row", gap: 10, marginHorizontal: 16, marginTop: 10 },
  libButton: { flex: 1, borderWidth: 1, borderColor: colors.border, backgroundColor: colors.card, borderRadius: 999, paddingVertical: 11, alignItems: "center" },
  libOn: { borderColor: "rgba(251,191,36,0.6)", backgroundColor: "rgba(251,191,36,0.12)" },
  libText: { color: colors.text, fontWeight: "700" },
  libTextOn: { color: colors.gold },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: 8, paddingHorizontal: 16, marginTop: 16 },
  chip: { color: colors.text, borderWidth: 1, borderColor: colors.border, borderRadius: 999, paddingHorizontal: 12, paddingVertical: 5, fontSize: 13, overflow: "hidden" },
  tagline: { color: colors.gold, fontStyle: "italic", paddingHorizontal: 16, marginTop: 16 },
  overview: { color: colors.mute, lineHeight: 22, paddingHorizontal: 16, marginTop: 10, fontSize: 15 },
  section: { color: colors.text, fontSize: 18, fontWeight: "700", marginLeft: 16, marginTop: 24, marginBottom: 12 },
  castItem: { width: 84 },
  castPhoto: { width: 84, height: 84, borderRadius: 42, overflow: "hidden", backgroundColor: colors.card, alignItems: "center", justifyContent: "center" },
  castInitial: { color: colors.mute, fontSize: 24, fontWeight: "700" },
  castName: { color: colors.text, fontSize: 12, fontWeight: "600", marginTop: 6, textAlign: "center" },
  castRole: { color: colors.dim, fontSize: 11, textAlign: "center" },
  credit: { color: colors.mute, paddingHorizontal: 16, marginTop: 20 },
});

import { Image } from "expo-image";
import { router } from "expo-router";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { posterUrl } from "@/lib/tmdb";
import { colors, radius } from "@/theme";
import type { MediaType, Movie } from "@/types/movie";

export function openTitle(movie: Movie, fallback: MediaType) {
  router.push({ pathname: "/title/[media]/[id]", params: { media: movie.media_type ?? fallback, id: String(movie.id) } });
}

export function PosterCard({ movie, mode, width = 118 }: { movie: Movie; mode: MediaType; width?: number }) {
  const uri = posterUrl(movie.poster_path, "w342");
  return (
    <Pressable onPress={() => openTitle(movie, mode)} style={({ pressed }) => [{ width, opacity: pressed ? 0.75 : 1 }]}>
      <View style={[styles.poster, { width, height: width * 1.5 }]}>
        {uri ? (
          <Image source={{ uri }} style={StyleSheet.absoluteFill} contentFit="cover" transition={200} recyclingKey={String(movie.id)} />
        ) : (
          <Text style={styles.fallback} numberOfLines={3}>
            {movie.title}
          </Text>
        )}
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  poster: { borderRadius: radius.sm, overflow: "hidden", backgroundColor: colors.card, justifyContent: "center" },
  fallback: { color: colors.mute, fontSize: 12, padding: 8, textAlign: "center" },
});

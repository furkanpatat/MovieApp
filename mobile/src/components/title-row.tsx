import { FlatList, StyleSheet, Text, View } from "react-native";

import { PosterCard } from "@/components/poster-card";
import { useLayout } from "@/lib/layout";
import { colors } from "@/theme";
import type { MediaType, Movie } from "@/types/movie";

/** A titled, horizontally scrolling row of posters (with a shimmer-free
 *  placeholder while loading). */
export function TitleRow({ title, items, mode, loading }: { title: string; items?: Movie[]; mode: MediaType; loading?: boolean }) {
  const { poster, gutter } = useLayout();
  return (
    <View style={styles.wrap}>
      <Text style={[styles.title, { marginLeft: gutter }]}>{title}</Text>
      {loading ? (
        <View style={[styles.skeletons, { paddingHorizontal: gutter }]}>
          {[0, 1, 2, 3].map((i) => (
            <View key={i} style={[styles.skeleton, { width: poster, height: poster * 1.5 }]} />
          ))}
        </View>
      ) : (
        <FlatList
          horizontal
          data={items?.filter((m) => m.poster_path) ?? []}
          keyExtractor={(m) => String(m.id)}
          renderItem={({ item }) => <PosterCard movie={item} mode={mode} width={poster} />}
          contentContainerStyle={{ paddingHorizontal: gutter }}
          ItemSeparatorComponent={() => <View style={{ width: 10 }} />}
          showsHorizontalScrollIndicator={false}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { marginTop: 22 },
  title: { color: colors.text, fontSize: 18, fontWeight: "700", marginBottom: 10 },
  skeletons: { flexDirection: "row", gap: 10 },
  skeleton: { borderRadius: 10, backgroundColor: colors.card },
});

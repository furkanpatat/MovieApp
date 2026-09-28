import { FlatList, StyleSheet, Text, View } from "react-native";

import { PosterCard } from "@/components/poster-card";
import { colors } from "@/theme";
import type { MediaType, Movie } from "@/types/movie";

/** A titled, horizontally scrolling row of posters (with a shimmer-free
 *  placeholder while loading). */
export function TitleRow({ title, items, mode, loading }: { title: string; items?: Movie[]; mode: MediaType; loading?: boolean }) {
  return (
    <View style={styles.wrap}>
      <Text style={styles.title}>{title}</Text>
      {loading ? (
        <View style={styles.skeletons}>
          {[0, 1, 2, 3].map((i) => (
            <View key={i} style={styles.skeleton} />
          ))}
        </View>
      ) : (
        <FlatList
          horizontal
          data={items?.filter((m) => m.poster_path) ?? []}
          keyExtractor={(m) => String(m.id)}
          renderItem={({ item }) => <PosterCard movie={item} mode={mode} />}
          contentContainerStyle={styles.list}
          ItemSeparatorComponent={() => <View style={{ width: 10 }} />}
          showsHorizontalScrollIndicator={false}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { marginTop: 22 },
  title: { color: colors.text, fontSize: 18, fontWeight: "700", marginLeft: 16, marginBottom: 10 },
  list: { paddingHorizontal: 16 },
  skeletons: { flexDirection: "row", gap: 10, paddingHorizontal: 16 },
  skeleton: { width: 118, height: 177, borderRadius: 10, backgroundColor: colors.card },
});

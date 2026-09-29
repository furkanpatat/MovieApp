import { useState } from "react";
import { ActivityIndicator, FlatList, StyleSheet, Text, TextInput, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { PosterCard } from "@/components/poster-card";
import { useT } from "@/i18n";
import { useLayout } from "@/lib/layout";
import { usePopular, useSearch } from "@/lib/queries";
import { useMode } from "@/store/mode";
import { colors, radius } from "@/theme";

/** Search the current mode (movies or series) as you type. */
export default function Search() {
  const mode = useMode((s) => s.mode);
  const { t } = useT();
  const [q, setQ] = useState("");
  const results = useSearch(mode, q);
  const popular = usePopular(mode);
  const searching = q.trim().length >= 2;
  const insets = useSafeAreaInsets();
  const { width, gutter, columns } = useLayout();
  const cardWidth = Math.floor((width - gutter * 2 - 10 * (columns - 1)) / columns);
  // Nothing typed yet: what's popular, so the screen is never empty.
  const items = (searching ? results.data?.results : popular.data?.results)?.filter((m) => m.poster_path) ?? [];

  return (
    <View style={[styles.screen, { paddingTop: insets.top + 12 }]}>
      <Text style={styles.heading}>{t.search.title}</Text>
      <TextInput
        value={q}
        onChangeText={setQ}
        placeholder={mode === "tv" ? t.search.series : t.search.movies}
        placeholderTextColor={colors.dim}
        style={styles.input}
        autoCorrect={false}
        autoCapitalize="none"
        returnKeyType="search"
        clearButtonMode="while-editing"
      />
      {results.isFetching && <ActivityIndicator color={colors.gold} style={{ marginTop: 16 }} />}
      <FlatList
        data={items}
        keyExtractor={(m) => String(m.id)}
        // A new column count needs a new list (FlatList can't change it live).
        key={columns}
        numColumns={columns}
        columnWrapperStyle={{ gap: 10 }}
        contentContainerStyle={{ gap: 10, paddingHorizontal: gutter, paddingTop: 16, paddingBottom: 120 }}
        renderItem={({ item }) => <PosterCard movie={item} mode={mode} width={cardWidth} />}
        keyboardDismissMode="on-drag"
        ListHeaderComponent={!searching && items.length > 0 ? <Text style={styles.section}>{t.search.trending}</Text> : null}
        ListEmptyComponent={searching && !results.isFetching ? <Text style={styles.empty}>{t.search.none(q.trim())}</Text> : null}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  heading: { color: colors.text, fontSize: 32, fontWeight: "800", marginHorizontal: 16, marginBottom: 12 },
  input: {
    marginHorizontal: 16,
    backgroundColor: colors.card,
    color: colors.text,
    borderRadius: radius.md,
    paddingHorizontal: 16,
    paddingVertical: 12,
    fontSize: 16,
    borderWidth: 1,
    borderColor: colors.border,
  },
  section: { color: colors.text, fontSize: 18, fontWeight: "700", marginBottom: 2 },
  empty: { color: colors.mute, textAlign: "center", marginTop: 32 },
});

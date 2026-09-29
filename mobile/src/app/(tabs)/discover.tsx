import { useFocusEffect } from "expo-router";
import { useCallback, useMemo, useState } from "react";
import { ActivityIndicator, FlatList, RefreshControl, StyleSheet, Text, useWindowDimensions, View, type ViewToken } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { FeedPost } from "@/components/feed-post";
import { useT } from "@/i18n";
import { useDiscoverFeed, useMyRatings } from "@/lib/queries";
import { useMode } from "@/store/mode";
import { colors } from "@/theme";
import type { Movie } from "@/types/movie";

/**
 * Discover: a vertical, full-screen trailer feed. One post per page; only
 * the one on screen plays, and only while this tab is focused. More pages
 * load as you near the end; pull down at the top for a fresh feed.
 */
export default function Discover() {
  const mode = useMode((s) => s.mode);
  const { t } = useT();
  // A new random start and order on each visit, and on pull-to-refresh.
  const [seed, setSeed] = useState(newSeed);
  const feed = useDiscoverFeed(mode, seed);
  const ratings = useMyRatings();
  const { height } = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const [active, setActive] = useState(0);
  const [focused, setFocused] = useState(true);

  useFocusEffect(
    useCallback(() => {
      setFocused(true);
      return () => setFocused(false);
    }, []),
  );

  // Pages can repeat a title; keep the first, and only titles with a picture.
  const items = useMemo(() => {
    const seen = new Set<number>();
    const out: Movie[] = [];
    for (const page of feed.data?.pages ?? []) {
      for (const m of page.results) {
        if (seen.has(m.id) || !(m.backdrop_path || m.poster_path)) continue;
        seen.add(m.id);
        out.push(m);
      }
    }
    return out;
  }, [feed.data]);

  const liked = useMemo(
    () => new Set((ratings.data?.items ?? []).filter((r) => r.rating >= 8).map((r) => `${r.movie.media_type ?? "movie"}:${r.movie.id}`)),
    [ratings.data],
  );

  // FlatList wants the same callback for its whole life.
  const [onViewable] = useState(() => ({ viewableItems }: { viewableItems: ViewToken<Movie>[] }) => {
    const first = viewableItems[0];
    if (first?.index != null) setActive(first.index);
  });

  if (feed.isPending) return <ActivityIndicator style={styles.center} color={colors.gold} />;
  if (feed.isError) return <Text style={[styles.center, styles.error]}>{t.discover.failed}</Text>;

  return (
    <View style={styles.screen}>
      <FlatList
        key={`${mode}-${seed}`}
        data={items}
        keyExtractor={(m) => `${m.media_type ?? mode}:${m.id}`}
        renderItem={({ item, index }) => (
          <FeedPost
            movie={item}
            mode={mode}
            height={height}
            active={focused && index === active}
            near={Math.abs(index - active) <= 1}
            liked={liked.has(`${item.media_type ?? mode}:${item.id}`)}
          />
        )}
        pagingEnabled
        contentInsetAdjustmentBehavior="never"
        automaticallyAdjustContentInsets={false}
        decelerationRate="fast"
        showsVerticalScrollIndicator={false}
        getItemLayout={(_, index) => ({ length: height, offset: height * index, index })}
        onViewableItemsChanged={onViewable}
        viewabilityConfig={VIEWABILITY}
        windowSize={3}
        initialNumToRender={2}
        maxToRenderPerBatch={2}
        onEndReached={() => feed.hasNextPage && !feed.isFetchingNextPage && void feed.fetchNextPage()}
        onEndReachedThreshold={2}
        refreshControl={
          <RefreshControl
            refreshing={false}
            onRefresh={() => (setSeed(newSeed()), setActive(0))}
            tintColor={colors.gold}
            colors={[colors.gold]}
            progressViewOffset={insets.top}
          />
        }
      />
      <View style={[styles.header, { top: insets.top + 8 }]} pointerEvents="none">
        <Text style={styles.headerText}>{t.discover.forYou}</Text>
        <Text style={styles.headerMode}>{mode === "tv" ? t.common.series : t.common.movies}</Text>
      </View>
    </View>
  );
}

const VIEWABILITY = { itemVisiblePercentThreshold: 60 };

const newSeed = () => Math.floor(Math.random() * 1_000_000);

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  center: { flex: 1, marginTop: 200, alignSelf: "center" },
  error: { color: colors.danger },
  header: { position: "absolute", left: 0, right: 0, alignItems: "center" },
  headerText: { color: colors.text, fontSize: 17, fontWeight: "800", textShadowColor: "rgba(0,0,0,0.6)", textShadowRadius: 6 },
  headerMode: { color: colors.gold, fontSize: 12, fontWeight: "700", marginTop: 2 },
});

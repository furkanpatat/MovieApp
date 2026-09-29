import * as Haptics from "expo-haptics";
import { useRef, useState } from "react";
import { Modal, Pressable, StyleSheet, Text, useWindowDimensions, View } from "react-native";
import Animated, { FadeIn } from "react-native-reanimated";

import { useT } from "@/i18n";
import { GENRES, genreLabel } from "@/lib/genres";
import { colors } from "@/theme";
import type { MediaType } from "@/types/movie";

const CARD_WIDTH = 288; // the web's w-72

/**
 * Discover's genre filter, the web's design: a translucent pill with the
 * current genre (the mode above it); tapping it opens a glass card right
 * under the pill (a quick fade, no motion) with the mode's genres in two
 * columns, the current one white and checked.
 */
export function GenrePicker({ mode, value, onChange }: { mode: MediaType; value: number; onChange: (id: number) => void }) {
  const { t, locale } = useT();
  const { width } = useWindowDimensions();
  const pill = useRef<View>(null);
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);
  const name = (id: number) => genreLabel(mode, id, locale);
  // Upper-cased for the locale (Turkish: i -> İ), not by textTransform.
  const up = (s: string) => s.toLocaleUpperCase(locale);

  const openCard = () => {
    void Haptics.selectionAsync();
    // Under the pill, centered on it, kept on screen.
    pill.current?.measureInWindow((x, y, w, h) => {
      const left = Math.max(12, Math.min(width - CARD_WIDTH - 12, x + w / 2 - CARD_WIDTH / 2));
      setAnchor({ x: left, y: y + h + 10 });
    });
  };
  const pick = (id: number) => {
    void Haptics.selectionAsync();
    setAnchor(null);
    if (id !== value) onChange(id);
  };

  return (
    <View style={styles.wrap}>
      <Text style={styles.mode}>{up(mode === "tv" ? t.common.series : t.common.movies)}</Text>
      <Pressable
        ref={pill}
        onPress={openCard}
        style={({ pressed }) => [styles.pill, pressed && { opacity: 0.8 }]}
        accessibilityRole="button"
        accessibilityLabel={t.discover.genre(name(value))}
        hitSlop={8}
      >
        <Text style={styles.pillText}>{name(value)}</Text>
        <Text style={[styles.chevron, anchor && styles.chevronOpen]}>⌄</Text>
      </Pressable>

      <Modal visible={!!anchor} transparent animationType="none" onRequestClose={() => setAnchor(null)} statusBarTranslucent>
        <Pressable style={StyleSheet.absoluteFill} onPress={() => setAnchor(null)} accessibilityLabel={t.discover.close} />
        {anchor && (
          <Animated.View entering={FadeIn.duration(140)} style={[styles.card, { left: anchor.x, top: anchor.y }]}>
            <Text style={styles.heading}>{up(mode === "tv" ? t.discover.seriesGenre : t.discover.movieGenre)}</Text>
            <View style={styles.grid}>
              {GENRES[mode].map((g) => {
                const on = g.id === value;
                return (
                  <Pressable
                    key={g.id}
                    onPress={() => pick(g.id)}
                    style={({ pressed }) => [styles.option, on && styles.optionOn, pressed && !on && styles.optionPressed]}
                    accessibilityRole="radio"
                    accessibilityState={{ checked: on }}
                  >
                    <Text style={[styles.optionText, on && styles.optionTextOn]} numberOfLines={1}>
                      {g.name[locale === "tr" ? 1 : 0]}
                    </Text>
                    {on && <Text style={styles.check}>✓</Text>}
                  </Pressable>
                );
              })}
            </View>
          </Animated.View>
        )}
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { alignItems: "center", gap: 6 },
  mode: { color: "rgba(255,255,255,0.6)", fontSize: 11, fontWeight: "700", letterSpacing: 2, textShadowColor: "rgba(0,0,0,0.7)", textShadowRadius: 6 },
  // bg-black/40, border-white/15, rounded-full, px-4 py-1.5, text-sm medium
  pill: { flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 16, paddingVertical: 7, borderRadius: 999, backgroundColor: "rgba(0,0,0,0.4)", borderWidth: 1, borderColor: "rgba(255,255,255,0.15)" },
  pillText: { color: colors.text, fontSize: 14, fontWeight: "600" },
  chevron: { color: "rgba(255,255,255,0.7)", fontSize: 15, fontWeight: "700", marginTop: -5 },
  chevronOpen: { transform: [{ rotate: "180deg" }], marginTop: 5 },
  // w-72, rounded-2xl, border-white/10, bg-zinc-950/90, p-3
  card: { position: "absolute", width: CARD_WIDTH, padding: 12, borderRadius: 16, backgroundColor: "rgba(9,9,11,0.94)", borderWidth: 1, borderColor: "rgba(255,255,255,0.1)", shadowColor: "#000", shadowOpacity: 0.6, shadowRadius: 30, shadowOffset: { width: 0, height: 16 }, elevation: 16 },
  heading: { color: "rgba(255,255,255,0.4)", fontSize: 11, fontWeight: "600", letterSpacing: 2.2, paddingHorizontal: 4, paddingBottom: 8 },
  grid: { flexDirection: "row", flexWrap: "wrap", gap: 6 },
  // two columns: (288 - 2 * 12 padding - 2 * 1 border - 6 gap) / 2
  option: { width: 128, flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 12, paddingVertical: 7, borderRadius: 999, backgroundColor: "rgba(255,255,255,0.04)" },
  optionOn: { backgroundColor: "#ffffff" },
  optionPressed: { backgroundColor: "rgba(255,255,255,0.1)" },
  optionText: { color: "rgba(255,255,255,0.7)", fontSize: 14, fontWeight: "500", flexShrink: 1 },
  optionTextOn: { color: "#000000", fontWeight: "700" },
  check: { color: "#000000", fontSize: 13, fontWeight: "900", marginLeft: 4 },
});

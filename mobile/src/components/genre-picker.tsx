import * as Haptics from "expo-haptics";
import { useState } from "react";
import { Modal, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import Animated, { FadeIn, SlideInDown } from "react-native-reanimated";
import { initialWindowMetrics } from "react-native-safe-area-context";

import { useT } from "@/i18n";
import { GENRES, genreLabel } from "@/lib/genres";
import { colors } from "@/theme";
import type { MediaType } from "@/types/movie";

/**
 * Discover's genre filter, as on the web: a pill with the current genre
 * (the mode, movies or series, above it); tapping it springs up a sheet:
 * "For You" (every genre) as its own card, then the mode's genres as
 * wrapping chips, the current one gold and checked.
 */
export function GenrePicker({ mode, value, onChange }: { mode: MediaType; value: number; onChange: (id: number) => void }) {
  const { t, locale } = useT();
  // The device's own bottom inset: inside a tab the context's includes the
  // tab bar, which a modal sheet covers.
  const bottomInset = initialWindowMetrics?.insets.bottom ?? 0;
  const [open, setOpen] = useState(false);
  const name = (i: number) => genreLabel(mode, i, locale);
  // Upper-cased for the locale (Turkish: i -> İ, not I), not by textTransform.
  const modeName = (mode === "tv" ? t.common.series : t.common.movies).toLocaleUpperCase(locale);

  const pick = (id: number) => {
    void Haptics.selectionAsync();
    setOpen(false);
    if (id !== value) onChange(id);
  };

  return (
    <View style={styles.anchor}>
      <Text style={styles.mode}>{modeName}</Text>
      <Pressable
        onPress={() => (void Haptics.selectionAsync(), setOpen(true))}
        style={({ pressed }) => [styles.pill, value !== 0 && styles.pillOn, pressed && { transform: [{ scale: 0.96 }] }]}
        accessibilityRole="button"
        accessibilityLabel={t.discover.genre(name(value))}
        hitSlop={8}
      >
        <Text style={[styles.pillText, value !== 0 && styles.pillTextOn]}>{name(value)}</Text>
        <Text style={[styles.chevron, value !== 0 && styles.pillTextOn]}>⌄</Text>
      </Pressable>

      <Modal visible={open} transparent animationType="none" onRequestClose={() => setOpen(false)} statusBarTranslucent>
        <Animated.View entering={FadeIn.duration(180)} style={styles.backdrop}>
          <Pressable style={StyleSheet.absoluteFill} onPress={() => setOpen(false)} accessibilityLabel={t.discover.close} />
        </Animated.View>
        <View style={styles.sheetAnchor} pointerEvents="box-none">
          <Animated.View entering={SlideInDown.springify().damping(22).stiffness(220)} style={[styles.sheet, { paddingBottom: bottomInset + 16 }]}>
            <View style={styles.grabber} />
            <View style={styles.head}>
              <View style={{ flex: 1 }}>
                <Text style={styles.kicker}>{modeName}</Text>
                <Text style={styles.title}>{t.discover.genres}</Text>
              </View>
              <Pressable onPress={() => setOpen(false)} hitSlop={12} style={styles.close} accessibilityLabel={t.discover.close}>
                <Text style={styles.closeText}>✕</Text>
              </Pressable>
            </View>

            <ScrollView showsVerticalScrollIndicator={false}>
              <Pressable
                onPress={() => pick(0)}
                style={({ pressed }) => [styles.forYou, value === 0 && styles.forYouOn, pressed && { opacity: 0.85 }]}
                accessibilityRole="radio"
                accessibilityState={{ checked: value === 0 }}
              >
                <Text style={[styles.forYouIcon, value === 0 && styles.onGold]}>✦</Text>
                <View style={{ flex: 1 }}>
                  <Text style={[styles.forYouTitle, value === 0 && styles.onGold]}>{t.discover.forYou}</Text>
                  <Text style={[styles.forYouSub, value === 0 && styles.onGoldDim]}>{t.discover.allGenres}</Text>
                </View>
                {value === 0 && <Text style={[styles.check, styles.onGold]}>✓</Text>}
              </Pressable>

              <View style={styles.chips}>
                {GENRES[mode]
                  .filter((g) => g.id !== 0)
                  .map((g) => {
                    const on = g.id === value;
                    return (
                      <Pressable
                        key={g.id}
                        onPress={() => pick(g.id)}
                        style={({ pressed }) => [styles.chip, on && styles.chipOn, pressed && { transform: [{ scale: 0.95 }] }]}
                        accessibilityRole="radio"
                        accessibilityState={{ checked: on }}
                      >
                        {on && <Text style={[styles.chipCheck, styles.onGold]}>✓</Text>}
                        <Text style={[styles.chipText, on && styles.onGold]}>{g.name[locale === "tr" ? 1 : 0]}</Text>
                      </Pressable>
                    );
                  })}
              </View>
            </ScrollView>
          </Animated.View>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  anchor: { alignItems: "center", gap: 6 },
  mode: { color: colors.gold, fontSize: 11, fontWeight: "800", letterSpacing: 1.6, textShadowColor: "rgba(0,0,0,0.7)", textShadowRadius: 6 },
  pill: { flexDirection: "row", alignItems: "center", gap: 6, paddingLeft: 16, paddingRight: 12, paddingVertical: 8, borderRadius: 999, backgroundColor: "rgba(9,9,11,0.55)", borderWidth: 1, borderColor: "rgba(255,255,255,0.2)" },
  pillOn: { backgroundColor: colors.gold, borderColor: colors.gold },
  pillText: { color: colors.text, fontSize: 15, fontWeight: "800" },
  pillTextOn: { color: colors.onGold },
  chevron: { color: colors.text, fontSize: 16, fontWeight: "800", marginTop: -6 },
  backdrop: { position: "absolute", top: 0, right: 0, bottom: 0, left: 0, backgroundColor: "rgba(0,0,0,0.6)" },
  sheetAnchor: { flex: 1, justifyContent: "flex-end" },
  sheet: { maxHeight: "78%", width: "100%", maxWidth: 720, alignSelf: "center", backgroundColor: colors.card, borderTopLeftRadius: 28, borderTopRightRadius: 28, paddingHorizontal: 18, paddingTop: 8, borderWidth: 1, borderColor: colors.border },
  grabber: { alignSelf: "center", width: 40, height: 5, borderRadius: 3, backgroundColor: colors.cardHi, marginBottom: 12 },
  head: { flexDirection: "row", alignItems: "center", marginBottom: 14 },
  kicker: { color: colors.gold, fontSize: 11, fontWeight: "800", letterSpacing: 1.6 },
  title: { color: colors.text, fontSize: 24, fontWeight: "800", marginTop: 2 },
  close: { width: 34, height: 34, borderRadius: 17, backgroundColor: colors.cardHi, alignItems: "center", justifyContent: "center" },
  closeText: { color: colors.text, fontSize: 15, fontWeight: "700" },
  forYou: { flexDirection: "row", alignItems: "center", gap: 14, padding: 16, borderRadius: 18, backgroundColor: colors.bg, borderWidth: 1, borderColor: colors.border, marginBottom: 16 },
  forYouOn: { backgroundColor: colors.gold, borderColor: colors.gold },
  forYouIcon: { color: colors.gold, fontSize: 22 },
  forYouTitle: { color: colors.text, fontSize: 17, fontWeight: "800" },
  forYouSub: { color: colors.mute, marginTop: 2 },
  check: { fontSize: 18, fontWeight: "900" },
  onGold: { color: colors.onGold },
  onGoldDim: { color: "rgba(9,9,11,0.7)" },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: 10 },
  chip: { flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 16, paddingVertical: 10, borderRadius: 999, backgroundColor: colors.bg, borderWidth: 1, borderColor: colors.border },
  chipOn: { backgroundColor: colors.gold, borderColor: colors.gold },
  chipCheck: { fontSize: 13, fontWeight: "900" },
  chipText: { color: colors.text, fontSize: 15, fontWeight: "700" },
});

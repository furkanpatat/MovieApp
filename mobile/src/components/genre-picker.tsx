import * as Haptics from "expo-haptics";
import { useState } from "react";
import { Modal, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { useT } from "@/i18n";
import { GENRES, genreLabel } from "@/lib/genres";
import { colors } from "@/theme";
import type { MediaType } from "@/types/movie";

/**
 * Discover's genre filter, as on the web: a pill with the current genre;
 * tapping it slides up a sheet of genres (the ones for movies or series).
 */
export function GenrePicker({ mode, value, onChange }: { mode: MediaType; value: number; onChange: (id: number) => void }) {
  const { t, locale } = useT();
  const insets = useSafeAreaInsets();
  const [open, setOpen] = useState(false);

  const pick = (id: number) => {
    void Haptics.selectionAsync();
    setOpen(false);
    if (id !== value) onChange(id);
  };

  return (
    <>
      <Pressable
        onPress={() => setOpen(true)}
        style={({ pressed }) => [styles.pill, value !== 0 && styles.pillOn, pressed && { opacity: 0.8 }]}
        accessibilityRole="button"
        accessibilityLabel={t.discover.genre(genreLabel(mode, value, locale))}
        hitSlop={8}
      >
        <Text style={[styles.pillText, value !== 0 && styles.pillTextOn]}>{genreLabel(mode, value, locale)}</Text>
        <Text style={[styles.chevron, value !== 0 && styles.pillTextOn]}>▾</Text>
      </Pressable>

      <Modal visible={open} transparent animationType="slide" onRequestClose={() => setOpen(false)}>
        <Pressable style={styles.backdrop} onPress={() => setOpen(false)} accessibilityLabel={t.discover.close} />
        <View style={[styles.sheet, { paddingBottom: insets.bottom + 16 }]}>
          <View style={styles.grabber} />
          <Text style={styles.title}>{t.discover.genres}</Text>
          <ScrollView contentContainerStyle={styles.grid}>
            {GENRES[mode].map((g) => {
              const on = g.id === value;
              return (
                <Pressable
                  key={g.id}
                  onPress={() => pick(g.id)}
                  style={({ pressed }) => [styles.chip, on && styles.chipOn, pressed && { opacity: 0.75 }]}
                  accessibilityRole="radio"
                  accessibilityState={{ checked: on }}
                >
                  <Text style={[styles.chipText, on && styles.chipTextOn]} numberOfLines={1}>
                    {g.name[locale === "tr" ? 1 : 0]}
                  </Text>
                </Pressable>
              );
            })}
          </ScrollView>
        </View>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  pill: { flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: 14, paddingVertical: 7, borderRadius: 999, backgroundColor: "rgba(0,0,0,0.45)", borderWidth: 1, borderColor: "rgba(255,255,255,0.18)" },
  pillOn: { backgroundColor: colors.gold, borderColor: colors.gold },
  pillText: { color: colors.text, fontSize: 15, fontWeight: "800" },
  pillTextOn: { color: colors.onGold },
  chevron: { color: colors.text, fontSize: 12, marginTop: 1 },
  backdrop: { flex: 1, backgroundColor: "rgba(0,0,0,0.55)" },
  sheet: { backgroundColor: colors.card, borderTopLeftRadius: 24, borderTopRightRadius: 24, paddingHorizontal: 16, paddingTop: 8, maxHeight: "70%", borderWidth: 1, borderColor: colors.border },
  grabber: { alignSelf: "center", width: 40, height: 5, borderRadius: 3, backgroundColor: colors.cardHi, marginBottom: 12 },
  title: { color: colors.text, fontSize: 18, fontWeight: "800", marginBottom: 12 },
  grid: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  chip: { width: "48.5%", paddingVertical: 12, paddingHorizontal: 14, borderRadius: 14, backgroundColor: colors.bg, borderWidth: 1, borderColor: colors.border },
  chipOn: { backgroundColor: colors.gold, borderColor: colors.gold },
  chipText: { color: colors.text, fontWeight: "700" },
  chipTextOn: { color: colors.onGold },
});

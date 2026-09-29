import { router } from "expo-router";
import { Pressable, ScrollView, Share, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { TitleRow } from "@/components/title-row";
import { API_URL } from "@/lib/api";
import { READABLE_WIDTH } from "@/lib/layout";
import { useLocale, useT } from "@/i18n";
import { useLibrary, useMyRatings } from "@/lib/queries";
import { logout } from "@/lib/session";
import { useAuth } from "@/store/auth";
import { colors, radius } from "@/theme";
import type { Movie } from "@/types/movie";

/** The public site, where /u/{username} is anyone's profile. */
const SITE = "https://kinora.duckdns.org";

/** Your account: what you watched (public on the web profile), My List,
 *  likes, and a link to share your profile. */
export default function Profile() {
  const username = useAuth((s) => s.username);
  const { t } = useT();
  const insets = useSafeAreaInsets();
  const { list, watched } = useLibrary();
  const ratings = useMyRatings();

  if (!username) {
    return (
      <View style={[styles.screen, { paddingTop: insets.top + 12 }]}>
        <View style={styles.column}>
        <Text style={styles.heading}>{t.profile.title}</Text>
        <View style={[styles.card, { marginHorizontal: 16 }]}>
          <Text style={styles.name}>{t.profile.pitch}</Text>
          <Text style={styles.sub}>{t.profile.pitchSub}</Text>
          <Pressable onPress={() => router.push("/login")} style={({ pressed }) => [styles.primary, pressed && { opacity: 0.8 }]}>
            <Text style={styles.primaryText}>{t.common.signIn}</Text>
          </Pressable>
        </View>
        <LanguagePicker />
        </View>
      </View>
    );
  }

  const watchedMovies: Movie[] = (watched.data?.items ?? []).map((w) => w.movie as Movie);
  const listMovies: Movie[] = (list.data?.items ?? []).map((w) => w.movie as Movie);
  const likes = (ratings.data?.items ?? []).filter((r) => r.rating >= 8).length;
  const profileUrl = `${SITE}/u/${encodeURIComponent(username)}`;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={[styles.column, { paddingTop: insets.top + 12, paddingBottom: insets.bottom + 110 }]}>
      <Text style={styles.heading}>{t.profile.title}</Text>
      <View style={[styles.card, { marginHorizontal: 16 }]}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>{username[0].toUpperCase()}</Text>
        </View>
        <Text style={styles.name}>{username}</Text>
        <View style={styles.stats}>
          <Stat value={watchedMovies.length} label={t.profile.watched} />
          <Stat value={listMovies.length} label={t.profile.myList} />
          <Stat value={likes} label={t.profile.liked} />
        </View>
        <Pressable
          onPress={() => void Share.share({ message: t.profile.shareMessage(profileUrl), url: profileUrl })}
          style={({ pressed }) => [styles.primary, pressed && { opacity: 0.8 }]}
        >
          <Text style={styles.primaryText}>{t.profile.share}</Text>
        </Pressable>
      </View>

      <TitleRow title={t.profile.watched} items={watchedMovies} mode="movie" loading={watched.isPending} />
      {!watched.isPending && watchedMovies.length === 0 && <Text style={styles.empty}>{t.profile.watchedEmpty}</Text>}
      <TitleRow title={t.profile.myList} items={listMovies} mode="movie" loading={list.isPending} />
      {!list.isPending && listMovies.length === 0 && <Text style={styles.empty}>{t.profile.listEmpty}</Text>}

      <LanguagePicker />
      <Pressable onPress={() => void logout()} style={({ pressed }) => [styles.secondary, pressed && { opacity: 0.8 }]}>
        <Text style={styles.secondaryText}>{t.profile.signOut}</Text>
      </Pressable>
      <Text style={styles.server}>{API_URL.replace(/^https?:\/\//, "")}</Text>
    </ScrollView>
  );
}

/** English | Türkçe, kept on the device. */
function LanguagePicker() {
  const { t, locale } = useT();
  const setLocale = useLocale((s) => s.setLocale);
  return (
    <View style={styles.langRow}>
      <Text style={styles.langLabel}>{t.profile.language}</Text>
      <View style={styles.segment}>
        {(["en", "tr"] as const).map((l) => (
          <Pressable key={l} onPress={() => setLocale(l)} style={[styles.segItem, locale === l && styles.segOn]} accessibilityRole="radio" accessibilityState={{ checked: locale === l }}>
            <Text style={[styles.segText, locale === l && styles.segTextOn]}>{l === "en" ? "English" : "Türkçe"}</Text>
          </Pressable>
        ))}
      </View>
    </View>
  );
}

function Stat({ value, label }: { value: number; label: string }) {
  return (
    <View style={styles.stat}>
      <Text style={styles.statValue}>{value}</Text>
      <Text style={styles.statLabel}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  column: { width: "100%", maxWidth: READABLE_WIDTH, alignSelf: "center" },
  heading: { color: colors.text, fontSize: 32, fontWeight: "800", marginBottom: 16, marginHorizontal: 16 },
  card: { backgroundColor: colors.card, borderRadius: radius.lg, padding: 20, alignItems: "center", borderWidth: 1, borderColor: colors.border },
  avatar: { width: 72, height: 72, borderRadius: 36, backgroundColor: colors.gold, alignItems: "center", justifyContent: "center" },
  avatarText: { color: colors.onGold, fontSize: 30, fontWeight: "900" },
  name: { color: colors.text, fontSize: 20, fontWeight: "700", marginTop: 12, textAlign: "center" },
  sub: { color: colors.mute, marginTop: 6, textAlign: "center" },
  stats: { flexDirection: "row", alignSelf: "stretch", justifyContent: "space-around", marginTop: 16 },
  stat: { alignItems: "center" },
  statValue: { color: colors.text, fontSize: 22, fontWeight: "800" },
  statLabel: { color: colors.mute, fontSize: 12, marginTop: 2 },
  primary: { marginTop: 18, backgroundColor: colors.gold, borderRadius: 999, paddingVertical: 12, alignSelf: "stretch", alignItems: "center" },
  primaryText: { color: colors.onGold, fontWeight: "800", fontSize: 16 },
  secondary: { marginTop: 28, marginHorizontal: 16, borderWidth: 1, borderColor: colors.border, borderRadius: 999, paddingVertical: 12, alignItems: "center" },
  secondaryText: { color: colors.text, fontWeight: "700" },
  empty: { color: colors.dim, marginHorizontal: 16, marginTop: -4 },
  langRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginHorizontal: 16, marginTop: 24 },
  langLabel: { color: colors.text, fontWeight: "700", fontSize: 16 },
  segment: { flexDirection: "row", backgroundColor: colors.card, borderRadius: 999, padding: 3, borderWidth: 1, borderColor: colors.border },
  segItem: { paddingHorizontal: 14, paddingVertical: 7, borderRadius: 999 },
  segOn: { backgroundColor: colors.gold },
  segText: { color: colors.mute, fontWeight: "700" },
  segTextOn: { color: colors.onGold },
  server: { color: colors.dim, textAlign: "center", marginTop: 16, fontSize: 12 },
});

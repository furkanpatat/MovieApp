import { router } from "expo-router";
import { Pressable, ScrollView, Share, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { TitleRow } from "@/components/title-row";
import { API_URL } from "@/lib/api";
import { useLibrary, useMyRatings } from "@/lib/queries";
import { useAuth } from "@/store/auth";
import { colors, radius } from "@/theme";
import type { Movie } from "@/types/movie";

/** The public site, where /u/{username} is anyone's profile. */
const SITE = "https://kinora.duckdns.org";

/** Your account: what you watched (public on the web profile), My List,
 *  likes, and a link to share your profile. */
export default function Profile() {
  const { username, signOut } = useAuth();
  const insets = useSafeAreaInsets();
  const { list, watched } = useLibrary();
  const ratings = useMyRatings();

  if (!username) {
    return (
      <View style={[styles.screen, { paddingTop: insets.top + 12 }]}>
        <Text style={styles.heading}>Profile</Text>
        <View style={[styles.card, { marginHorizontal: 16 }]}>
          <Text style={styles.name}>Your list, ratings and watch parties</Text>
          <Text style={styles.sub}>Sign in with your KinoCut account (the same one as on the web).</Text>
          <Pressable onPress={() => router.push("/login")} style={({ pressed }) => [styles.primary, pressed && { opacity: 0.8 }]}>
            <Text style={styles.primaryText}>Sign in</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  const watchedMovies: Movie[] = (watched.data?.items ?? []).map((w) => w.movie as Movie);
  const listMovies: Movie[] = (list.data?.items ?? []).map((w) => w.movie as Movie);
  const likes = (ratings.data?.items ?? []).filter((r) => r.rating >= 8).length;
  const profileUrl = `${SITE}/u/${encodeURIComponent(username)}`;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={{ paddingTop: insets.top + 12, paddingBottom: insets.bottom + 110 }}>
      <Text style={styles.heading}>Profile</Text>
      <View style={[styles.card, { marginHorizontal: 16 }]}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>{username[0].toUpperCase()}</Text>
        </View>
        <Text style={styles.name}>{username}</Text>
        <View style={styles.stats}>
          <Stat value={watchedMovies.length} label="Watched" />
          <Stat value={listMovies.length} label="My List" />
          <Stat value={likes} label="Liked" />
        </View>
        <Pressable
          onPress={() => void Share.share({ message: `What I watch on KinoCut: ${profileUrl}`, url: profileUrl })}
          style={({ pressed }) => [styles.primary, pressed && { opacity: 0.8 }]}
        >
          <Text style={styles.primaryText}>Share my profile</Text>
        </Pressable>
      </View>

      <TitleRow title="Watched" items={watchedMovies} mode="movie" loading={watched.isPending} />
      {!watched.isPending && watchedMovies.length === 0 && <Text style={styles.empty}>Mark titles as watched from their details.</Text>}
      <TitleRow title="My List" items={listMovies} mode="movie" loading={list.isPending} />
      {!list.isPending && listMovies.length === 0 && <Text style={styles.empty}>Save titles to watch later from their details.</Text>}

      <Pressable onPress={() => void signOut()} style={({ pressed }) => [styles.secondary, pressed && { opacity: 0.8 }]}>
        <Text style={styles.secondaryText}>Sign out</Text>
      </Pressable>
      <Text style={styles.server}>{API_URL.replace(/^https?:\/\//, "")}</Text>
    </ScrollView>
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
  server: { color: colors.dim, textAlign: "center", marginTop: 16, fontSize: 12 },
});

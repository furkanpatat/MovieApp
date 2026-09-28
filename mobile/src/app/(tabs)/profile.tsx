import { router } from "expo-router";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { API_URL } from "@/lib/api";
import { useAuth } from "@/store/auth";
import { colors, radius } from "@/theme";

export default function Profile() {
  const { username, signOut } = useAuth();
  const insets = useSafeAreaInsets();

  return (
    <View style={[styles.screen, { paddingTop: insets.top + 12 }]}>
      <Text style={styles.heading}>Profile</Text>
      {username ? (
        <View style={styles.card}>
          <View style={styles.avatar}>
            <Text style={styles.avatarText}>{username[0].toUpperCase()}</Text>
          </View>
          <Text style={styles.name}>{username}</Text>
          <Pressable onPress={() => void signOut()} style={({ pressed }) => [styles.secondary, pressed && { opacity: 0.8 }]}>
            <Text style={styles.secondaryText}>Sign out</Text>
          </Pressable>
        </View>
      ) : (
        <View style={styles.card}>
          <Text style={styles.name}>Your list, ratings and watch parties</Text>
          <Text style={styles.sub}>Sign in with your KinoCut account (the same one as on the web).</Text>
          <Pressable onPress={() => router.push("/login")} style={({ pressed }) => [styles.primary, pressed && { opacity: 0.8 }]}>
            <Text style={styles.primaryText}>Sign in</Text>
          </Pressable>
        </View>
      )}
      <Text style={styles.server}>{API_URL.replace(/^https?:\/\//, "")}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg, paddingHorizontal: 16 },
  heading: { color: colors.text, fontSize: 32, fontWeight: "800", marginBottom: 16 },
  card: { backgroundColor: colors.card, borderRadius: radius.lg, padding: 20, alignItems: "center", borderWidth: 1, borderColor: colors.border },
  avatar: { width: 72, height: 72, borderRadius: 36, backgroundColor: colors.gold, alignItems: "center", justifyContent: "center" },
  avatarText: { color: colors.onGold, fontSize: 30, fontWeight: "900" },
  name: { color: colors.text, fontSize: 20, fontWeight: "700", marginTop: 12, textAlign: "center" },
  sub: { color: colors.mute, marginTop: 6, textAlign: "center" },
  primary: { marginTop: 18, backgroundColor: colors.gold, borderRadius: 999, paddingVertical: 12, alignSelf: "stretch", alignItems: "center" },
  primaryText: { color: colors.onGold, fontWeight: "800", fontSize: 16 },
  secondary: { marginTop: 18, borderWidth: 1, borderColor: colors.border, borderRadius: 999, paddingVertical: 12, alignSelf: "stretch", alignItems: "center" },
  secondaryText: { color: colors.text, fontWeight: "700" },
  server: { color: colors.dim, textAlign: "center", marginTop: 24, fontSize: 12 },
});

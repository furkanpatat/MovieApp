import { router, Stack, useLocalSearchParams } from "expo-router";
import { useState } from "react";
import { FlatList, KeyboardAvoidingView, Pressable, Share, StyleSheet, Text, TextInput, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { PartyPlayer } from "@/components/party-player";
import { displayName, inviteUrl, parsePartyCode, partyRoom } from "@/lib/party";
import { useTitle } from "@/lib/queries";
import { useWatchParty } from "@/lib/use-watch-party";
import { useAuth } from "@/store/auth";
import { colors, radius } from "@/theme";

function clock(seconds: number) {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/**
 * A Watch Party on the phone: the same rooms as the web (a movie's open
 * room, or a private one from an invite), so a friend on the site and you
 * here watch the trailer in sync and chat.
 */
export default function PartyScreen() {
  const params = useLocalSearchParams<{ id: string; code?: string }>();
  const movieId = Number(params.id);
  const code = parsePartyCode(params.code);
  const signedIn = useAuth((s) => !!s.token);

  if (!signedIn) {
    return (
      <View style={[styles.screen, styles.center]}>
        <Text style={styles.title}>Watch together</Text>
        <Text style={styles.mute}>Sign in to join a watch party.</Text>
        <Pressable onPress={() => router.push("/login")} style={styles.primary}>
          <Text style={styles.primaryText}>Sign in</Text>
        </Pressable>
      </View>
    );
  }
  return <Party movieId={movieId} code={code} />;
}

function Party({ movieId, code }: { movieId: number; code: string | null }) {
  const insets = useSafeAreaInsets();
  const movie = useTitle("movie", movieId);
  const wp = useWatchParty(partyRoom(movieId, code));
  const [draft, setDraft] = useState("");
  const trailer = movie.data?.trailer_key;

  // On return, the input's own text: the draft state can lag a fast typist.
  const send = (typed?: string) => {
    const text = (typed ?? draft).trim();
    if (!text) return;
    wp.sendChat(text);
    setDraft("");
  };

  const statusLine =
    wp.status === "open"
      ? wp.lastEvent
        ? `${displayName(wp.lastEvent.user_id, wp.me)} ${wp.lastEvent.action === "pause" ? "paused" : "pressed play"} at ${clock(wp.lastEvent.timestamp)}`
        : "Press play to start it for everyone."
      : wp.status === "connecting"
        ? "Connecting…"
        : "Not connected.";

  return (
    <KeyboardAvoidingView behavior="padding" keyboardVerticalOffset={insets.top + 44} style={styles.screen}>
      <Stack.Screen options={{ headerTitle: movie.data?.title ?? "Watch Party" }} />
      <View style={styles.top}>
        <View style={styles.badges}>
          <Text style={[styles.badge, code ? styles.private : styles.open]}>{code ? "🔒 Private party" : "🌐 Open room"}</Text>
          <Text style={styles.people}>👥 {wp.people}</Text>
          <View style={[styles.dot, { backgroundColor: wp.status === "open" ? "#22c55e" : wp.status === "connecting" ? colors.gold : colors.danger }]} />
        </View>

        {trailer ? (
          <PartyPlayer videoKey={trailer} playback={wp.playback} onLocal={wp.sendPlayback} />
        ) : (
          <View style={[styles.noTrailer]}>
            <Text style={styles.mute}>{movie.isPending ? "Loading…" : "This title has no trailer to watch together."}</Text>
          </View>
        )}
        <Text style={styles.status}>{statusLine}</Text>

        <View style={styles.actions}>
          {code && (
            <Pressable
              onPress={() => void Share.share({ message: `Watch with me on KinoCut: ${inviteUrl(movieId, code)}`, url: inviteUrl(movieId, code) })}
              style={({ pressed }) => [styles.primary, styles.flex, pressed && { opacity: 0.8 }]}
            >
              <Text style={styles.primaryText}>Invite friends</Text>
            </Pressable>
          )}
          {(wp.status === "closed" || wp.status === "error") && (
            <Pressable onPress={wp.connect} style={({ pressed }) => [styles.secondary, styles.flex, pressed && { opacity: 0.8 }]}>
              <Text style={styles.secondaryText}>Reconnect</Text>
            </Pressable>
          )}
        </View>
      </View>

      <FlatList
        data={wp.feed}
        keyExtractor={(f) => f.id}
        style={styles.chat}
        contentContainerStyle={{ padding: 16, gap: 8 }}
        ListEmptyComponent={<Text style={styles.mute}>Say hi — messages go to everyone in the room.</Text>}
        renderItem={({ item }) =>
          item.kind === "system" ? (
            <Text style={styles.system}>{item.text}</Text>
          ) : (
            <View style={[styles.msg, item.userId === wp.me && styles.mine]}>
              <Text style={styles.who}>{displayName(item.userId, wp.me)}</Text>
              <Text style={styles.msgText}>{item.text}</Text>
            </View>
          )
        }
      />

      <View style={[styles.composer, { paddingBottom: insets.bottom + 8 }]}>
        <TextInput
          value={draft}
          onChangeText={setDraft}
          placeholder="Send a message…"
          placeholderTextColor={colors.dim}
          style={styles.input}
          onSubmitEditing={(e) => send(e.nativeEvent.text)}
          returnKeyType="send"
          maxLength={500}
          editable={wp.status === "open"}
        />
        <Pressable onPress={() => send()} disabled={!draft.trim() || wp.status !== "open"} style={({ pressed }) => [styles.send, (!draft.trim() || wp.status !== "open") && { opacity: 0.4 }, pressed && { opacity: 0.7 }]}>
          <Text style={styles.sendText}>↑</Text>
        </Pressable>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  center: { alignItems: "center", justifyContent: "center", padding: 24, gap: 8 },
  title: { color: colors.text, fontSize: 24, fontWeight: "800" },
  mute: { color: colors.mute, textAlign: "center" },
  top: { padding: 16, paddingBottom: 8 },
  badges: { flexDirection: "row", alignItems: "center", gap: 10, marginBottom: 12 },
  badge: { fontSize: 12, fontWeight: "700", paddingHorizontal: 10, paddingVertical: 4, borderRadius: 999, overflow: "hidden" },
  private: { color: colors.gold, backgroundColor: "rgba(251,191,36,0.14)" },
  open: { color: colors.text, backgroundColor: colors.cardHi },
  people: { color: colors.mute, fontWeight: "600" },
  dot: { width: 8, height: 8, borderRadius: 4, marginLeft: "auto" },
  noTrailer: { aspectRatio: 16 / 9, borderRadius: 14, backgroundColor: colors.card, alignItems: "center", justifyContent: "center", padding: 16 },
  status: { color: colors.text, marginTop: 4, fontWeight: "600" },
  actions: { flexDirection: "row", gap: 10, marginTop: 12 },
  flex: { flex: 1 },
  primary: { backgroundColor: colors.gold, borderRadius: 999, paddingVertical: 12, paddingHorizontal: 20, alignItems: "center" },
  primaryText: { color: colors.onGold, fontWeight: "800" },
  secondary: { borderWidth: 1, borderColor: colors.border, borderRadius: 999, paddingVertical: 12, alignItems: "center" },
  secondaryText: { color: colors.text, fontWeight: "700" },
  chat: { flex: 1, borderTopWidth: 1, borderTopColor: colors.border },
  system: { color: colors.dim, fontSize: 12, textAlign: "center" },
  msg: { alignSelf: "flex-start", maxWidth: "85%", backgroundColor: colors.card, borderRadius: radius.md, paddingHorizontal: 12, paddingVertical: 8 },
  mine: { alignSelf: "flex-end", backgroundColor: "rgba(251,191,36,0.16)" },
  who: { color: colors.gold, fontSize: 11, fontWeight: "700" },
  msgText: { color: colors.text, fontSize: 15, marginTop: 2 },
  composer: { flexDirection: "row", gap: 8, paddingHorizontal: 16, paddingTop: 8, borderTopWidth: 1, borderTopColor: colors.border },
  input: { flex: 1, backgroundColor: colors.card, color: colors.text, borderRadius: 22, paddingHorizontal: 16, paddingVertical: 11, fontSize: 16, borderWidth: 1, borderColor: colors.border },
  send: { width: 44, height: 44, borderRadius: 22, backgroundColor: colors.gold, alignItems: "center", justifyContent: "center" },
  sendText: { color: colors.onGold, fontSize: 22, fontWeight: "900" },
});

import { router } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Alert, FlatList, KeyboardAvoidingView, Modal, Platform, Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import Animated, { FadeIn } from "react-native-reanimated";
import { initialWindowMetrics } from "react-native-safe-area-context";

import { useT } from "@/i18n";
import { useBlockedUsers, useBlockUser, useComment, useInteractions, useReportComment } from "@/lib/queries";
import { displayName } from "@/lib/party";
import { useAuth } from "@/store/auth";
import { colors } from "@/theme";
import type { MediaType } from "@/types/movie";

const MAX_COMMENT = 1000; // the Interaction service's limit (runes)

/** A title's comments, in a sheet over the feed, with a composer. */
export function CommentsSheet({ media, id, title, open, onClose }: { media: MediaType; id: number; title: string; open: boolean; onClose: () => void }) {
  const { t } = useT();
  // The device's own bottom inset: inside a tab the context's includes the
  // tab bar, which a modal sheet covers.
  const bottomInset = initialWindowMetrics?.insets.bottom ?? 0;
  const me = useAuth((s) => s.userId);
  const signedIn = useAuth((s) => !!s.token);
  const q = useInteractions(media, id, open);
  const comment = useComment(media, id);
  const [draft, setDraft] = useState("");
  const blocked = useBlockedUsers().data;
  const report = useReportComment();
  const blockUser = useBlockUser();
  const comments = (q.data?.recent_comments ?? []).filter((c) => !blocked?.includes(c.user_id));

  // Report a comment, or block its author (which hides all their comments).
  const moderate = (c: { id: string; user_id: string }) => {
    if (!signedIn) return void (onClose(), router.push("/login"));
    Alert.alert(t.discover.more, undefined, [
      {
        text: t.discover.report,
        onPress: () =>
          report.mutate(c.id, {
            onSuccess: () => Alert.alert(t.discover.reported),
            onError: () => Alert.alert(t.discover.reportFailed),
          }),
      },
      {
        text: t.discover.block,
        style: "destructive",
        onPress: () =>
          blockUser.mutate(
            { target: c.user_id, block: true },
            {
              onSuccess: () =>
                Alert.alert(t.discover.blocked, undefined, [
                  { text: t.discover.undo, onPress: () => blockUser.mutate({ target: c.user_id, block: false }) },
                  { text: t.discover.ok, style: "cancel" },
                ]),
              onError: () => Alert.alert(t.discover.blockFailed),
            },
          ),
      },
      { text: t.discover.close, style: "cancel" },
    ]);
  };

  // Relative to when the list was fetched (render stays pure).
  const ago = (iso: string) => {
    const s = Math.max(0, (q.dataUpdatedAt - Date.parse(iso)) / 1000);
    if (!Number.isFinite(s) || s < 60) return t.discover.justNow;
    if (s < 3600) return t.discover.ago(Math.floor(s / 60), "m");
    if (s < 86400) return t.discover.ago(Math.floor(s / 3600), "h");
    return t.discover.ago(Math.floor(s / 86400), "d");
  };

  const send = (typed?: string) => {
    const text = (typed ?? draft).trim();
    if (!text || comment.isPending) return;
    setDraft("");
    comment.mutate(text, { onError: () => setDraft(text) });
  };

  return (
    <Modal visible={open} transparent animationType="none" onRequestClose={onClose} statusBarTranslucent>
      <Animated.View entering={FadeIn.duration(180)} style={styles.backdrop}>
        <Pressable style={StyleSheet.absoluteFill} onPress={onClose} accessibilityLabel={t.discover.close} />
      </Animated.View>
      <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={styles.anchor} pointerEvents="box-none">
        <Animated.View entering={FadeIn.duration(160)} style={[styles.sheet, { paddingBottom: bottomInset + 12 }]}>
          <View style={styles.grabber} />
          <View style={styles.head}>
            <View style={{ flex: 1 }}>
              <Text style={styles.title}>{t.discover.comments}</Text>
              <Text style={styles.sub} numberOfLines={1}>
                {title} · {t.discover.commentsCount(comments.length)}
              </Text>
            </View>
            <Pressable onPress={onClose} hitSlop={12} style={styles.close} accessibilityLabel={t.discover.close}>
              <Text style={styles.closeText}>✕</Text>
            </Pressable>
          </View>

          {q.isPending ? (
            <ActivityIndicator color={colors.gold} style={{ marginVertical: 32 }} />
          ) : (
            <FlatList
              data={comments}
              keyExtractor={(c) => c.id}
              style={styles.list}
              contentContainerStyle={{ gap: 14, paddingVertical: 8 }}
              keyboardShouldPersistTaps="handled"
              ListEmptyComponent={<Text style={styles.empty}>{t.discover.noComments}</Text>}
              renderItem={({ item }) => {
                const pending = (item as { pending?: boolean }).pending;
                return (
                  <View style={[styles.row, pending && { opacity: 0.55 }]}>
                    <View style={[styles.avatar, item.user_id === me && styles.avatarMe]}>
                      <Text style={styles.avatarText}>{(item.user_id === me ? t.common.you : item.user_id).slice(0, 1).toUpperCase()}</Text>
                    </View>
                    <View style={{ flex: 1 }}>
                      <Text style={styles.who}>
                        {displayName(item.user_id, me, t)}
                        <Text style={styles.when}>  ·  {pending ? t.discover.sending : ago(item.created_at)}</Text>
                      </Text>
                      <Text style={styles.text}>{item.text}</Text>
                    </View>
                    {!pending && item.user_id !== me && (
                      <Pressable onPress={() => moderate(item)} hitSlop={10} style={styles.more} accessibilityLabel={t.discover.more}>
                        <Text style={styles.moreText}>⋯</Text>
                      </Pressable>
                    )}
                  </View>
                );
              }}
            />
          )}

          {comment.isError && <Text style={styles.error}>{t.discover.commentFailed}</Text>}
          {signedIn ? (
            <View style={styles.composer}>
              <TextInput
                value={draft}
                onChangeText={setDraft}
                placeholder={t.discover.addComment}
                placeholderTextColor={colors.dim}
                style={styles.input}
                maxLength={MAX_COMMENT}
                multiline
                submitBehavior="submit"
                returnKeyType="send"
                onSubmitEditing={(e) => send(e.nativeEvent.text)}
              />
              <Pressable
                onPress={() => send()}
                disabled={!draft.trim() || comment.isPending}
                style={({ pressed }) => [styles.send, (!draft.trim() || comment.isPending) && { opacity: 0.4 }, pressed && { opacity: 0.7 }]}
                accessibilityLabel={t.discover.send}
              >
                <Text style={styles.sendText}>↑</Text>
              </Pressable>
            </View>
          ) : (
            <Pressable onPress={() => (onClose(), router.push("/login"))} style={styles.signIn}>
              <Text style={styles.signInText}>{t.discover.signInToComment}</Text>
            </Pressable>
          )}
        </Animated.View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { position: "absolute", top: 0, right: 0, bottom: 0, left: 0, backgroundColor: "rgba(0,0,0,0.6)" },
  anchor: { flex: 1, justifyContent: "flex-end" },
  sheet: { maxHeight: "72%", width: "100%", maxWidth: 720, alignSelf: "center", backgroundColor: colors.card, borderTopLeftRadius: 26, borderTopRightRadius: 26, paddingHorizontal: 18, paddingTop: 8, borderWidth: 1, borderColor: colors.border },
  grabber: { alignSelf: "center", width: 40, height: 5, borderRadius: 3, backgroundColor: colors.cardHi, marginBottom: 10 },
  head: { flexDirection: "row", alignItems: "center", marginBottom: 6 },
  title: { color: colors.text, fontSize: 19, fontWeight: "800" },
  sub: { color: colors.mute, marginTop: 2 },
  close: { width: 34, height: 34, borderRadius: 17, backgroundColor: colors.cardHi, alignItems: "center", justifyContent: "center" },
  closeText: { color: colors.text, fontSize: 15, fontWeight: "700" },
  list: { flexGrow: 0, minHeight: 120 },
  empty: { color: colors.mute, textAlign: "center", marginVertical: 28 },
  row: { flexDirection: "row", gap: 12 },
  avatar: { width: 34, height: 34, borderRadius: 17, backgroundColor: colors.cardHi, alignItems: "center", justifyContent: "center" },
  avatarMe: { backgroundColor: colors.gold },
  avatarText: { color: colors.text, fontWeight: "800" },
  who: { color: colors.text, fontWeight: "700", fontSize: 13 },
  when: { color: colors.dim, fontWeight: "400" },
  text: { color: "#e4e4e7", fontSize: 15, marginTop: 2, lineHeight: 20 },
  more: { width: 32, height: 32, alignItems: "center", justifyContent: "center" },
  moreText: { color: colors.mute, fontSize: 20, fontWeight: "800" },
  error: { color: colors.danger, marginTop: 6 },
  composer: { flexDirection: "row", alignItems: "flex-end", gap: 8, marginTop: 10 },
  input: { flex: 1, maxHeight: 110, backgroundColor: colors.bg, color: colors.text, borderRadius: 22, paddingHorizontal: 16, paddingTop: 11, paddingBottom: 11, fontSize: 16, borderWidth: 1, borderColor: colors.border },
  send: { width: 44, height: 44, borderRadius: 22, backgroundColor: colors.gold, alignItems: "center", justifyContent: "center" },
  sendText: { color: colors.onGold, fontSize: 22, fontWeight: "900" },
  signIn: { marginTop: 10, backgroundColor: colors.gold, borderRadius: 999, paddingVertical: 13, alignItems: "center" },
  signInText: { color: colors.onGold, fontWeight: "800" },
});

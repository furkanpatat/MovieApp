import * as Haptics from "expo-haptics";
import { router } from "expo-router";
import { useRef, useState } from "react";
import { ActivityIndicator, FlatList, KeyboardAvoidingView, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { Markdown } from "@/components/markdown";
import { PosterCard } from "@/components/poster-card";
import { ApiError } from "@/lib/api";
import { useT } from "@/i18n";
import { useChat, type ChatMessage } from "@/lib/queries";
import { useAuth } from "@/store/auth";
import { colors, radius } from "@/theme";
import type { Movie } from "@/types/movie";

interface Turn extends ChatMessage {
  id: number;
  movies?: Movie[];
  failed?: boolean;
}

/** The AI concierge: a chat whose picks are real, tappable titles. */
export default function Concierge() {
  const { t: all, locale } = useT();
  const t = all.concierge;
  const insets = useSafeAreaInsets();
  const signedIn = useAuth((s) => !!s.token);
  const chat = useChat();
  const [turns, setTurns] = useState<Turn[]>([]);
  const [draft, setDraft] = useState("");
  const nextId = useRef(1);
  const list = useRef<ScrollView>(null);

  const send = (text: string, before: Turn[] = turns) => {
    const content = text.trim();
    if (!content || chat.isPending) return;
    void Haptics.selectionAsync();
    const history: Turn[] = [...before.filter((x) => !x.failed), { id: nextId.current++, role: "user", content }];
    setTurns(history);
    setDraft("");
    chat.mutate(
      { messages: history.map(({ role, content: c }) => ({ role, content: c })), locale },
      {
        onSuccess: (r) => setTurns((cur) => [...cur, { id: nextId.current++, role: "assistant", content: r.message, movies: r.movies }]),
        onError: (e) =>
          setTurns((cur) => [
            ...cur,
            { id: nextId.current++, role: "assistant", content: e instanceof ApiError && e.status === 429 ? t.limited : t.failed, failed: true },
          ]),
      },
    );
  };

  const retry = () => {
    const lastUser = [...turns].reverse().find((x) => x.role === "user");
    if (!lastUser) return;
    const before = turns.slice(0, turns.indexOf(lastUser));
    send(lastUser.content, before);
  };

  return (
    <KeyboardAvoidingView behavior="padding" style={[styles.screen, { paddingTop: insets.top + 12 }]}>
      <View style={styles.head}>
        <Text style={styles.heading}>✨ {t.title}</Text>
        <Text style={styles.sub}>{t.sub}</Text>
      </View>

      <ScrollView
        ref={list}
        style={{ flex: 1 }}
        contentContainerStyle={styles.thread}
        onContentSizeChange={() => list.current?.scrollToEnd({ animated: true })}
        keyboardDismissMode="interactive"
      >
        {turns.length === 0 && signedIn && (
          <View style={styles.suggestions}>
            {t.suggestions.map((s) => (
              <Pressable key={s} onPress={() => send(s)} style={({ pressed }) => [styles.chip, pressed && { opacity: 0.7 }]}>
                <Text style={styles.chipText}>{s}</Text>
              </Pressable>
            ))}
          </View>
        )}
        {turns.map((turn) =>
          turn.role === "user" ? (
            <View key={turn.id} style={styles.userBubble}>
              <Text style={styles.userText}>{turn.content}</Text>
            </View>
          ) : (
            <View key={turn.id} style={[styles.botBubble, turn.failed && styles.failedBubble]}>
              <Markdown text={turn.content} color={turn.failed ? colors.danger : colors.text} />
              {turn.failed && (
                <Pressable onPress={retry} hitSlop={8}>
                  <Text style={styles.retry}>{t.retry}</Text>
                </Pressable>
              )}
              {turn.movies && turn.movies.length > 0 && (
                <FlatList
                  horizontal
                  data={turn.movies}
                  keyExtractor={(m) => String(m.id)}
                  renderItem={({ item }) => <PosterCard movie={item} mode={item.media_type ?? "movie"} width={96} />}
                  ItemSeparatorComponent={() => <View style={{ width: 8 }} />}
                  showsHorizontalScrollIndicator={false}
                  style={{ marginTop: 12 }}
                />
              )}
            </View>
          ),
        )}
        {chat.isPending && (
          <View style={[styles.botBubble, { alignSelf: "flex-start" }]}>
            <ActivityIndicator color={colors.gold} />
          </View>
        )}
      </ScrollView>

      {signedIn ? (
        <View style={[styles.composer, { marginBottom: insets.bottom + 70 }]}>
          <TextInput
            value={draft}
            onChangeText={setDraft}
            placeholder={t.placeholder}
            placeholderTextColor={colors.dim}
            style={styles.input}
            multiline
            maxLength={500}
            // The input's own text: the draft state can lag a fast typist.
            onSubmitEditing={(e) => send(e.nativeEvent.text)}
            submitBehavior="submit"
            returnKeyType="send"
          />
          <Pressable
            onPress={() => send(draft)}
            disabled={!draft.trim() || chat.isPending}
            style={({ pressed }) => [styles.send, (!draft.trim() || chat.isPending) && { opacity: 0.4 }, pressed && { opacity: 0.7 }]}
            accessibilityLabel={t.send}
          >
            <Text style={styles.sendText}>↑</Text>
          </Pressable>
        </View>
      ) : (
        <Pressable onPress={() => router.push("/login")} style={[styles.signIn, { marginBottom: insets.bottom + 76 }]}>
          <Text style={styles.signInText}>{t.signIn}</Text>
        </Pressable>
      )}
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  head: { paddingHorizontal: 16, paddingBottom: 8 },
  heading: { color: colors.text, fontSize: 32, fontWeight: "800" },
  sub: { color: colors.mute, marginTop: 4 },
  thread: { padding: 16, gap: 12 },
  suggestions: { gap: 10, marginTop: 8 },
  chip: { borderWidth: 1, borderColor: colors.border, backgroundColor: colors.card, borderRadius: radius.md, paddingHorizontal: 14, paddingVertical: 12 },
  chipText: { color: colors.text, fontSize: 15 },
  userBubble: { alignSelf: "flex-end", maxWidth: "85%", backgroundColor: colors.gold, borderRadius: 18, borderBottomRightRadius: 6, paddingHorizontal: 14, paddingVertical: 10 },
  userText: { color: colors.onGold, fontSize: 15, fontWeight: "600" },
  botBubble: { alignSelf: "stretch", backgroundColor: colors.card, borderRadius: 18, borderBottomLeftRadius: 6, padding: 14, borderWidth: 1, borderColor: colors.border },
  failedBubble: { borderColor: "rgba(239,68,68,0.4)" },
  retry: { color: colors.gold, fontWeight: "700", marginTop: 8 },
  composer: { flexDirection: "row", alignItems: "flex-end", gap: 8, marginHorizontal: 16 },
  input: { flex: 1, maxHeight: 120, backgroundColor: colors.card, color: colors.text, borderRadius: 22, paddingHorizontal: 16, paddingTop: 12, paddingBottom: 12, fontSize: 16, borderWidth: 1, borderColor: colors.border },
  send: { width: 44, height: 44, borderRadius: 22, backgroundColor: colors.gold, alignItems: "center", justifyContent: "center" },
  sendText: { color: colors.onGold, fontSize: 22, fontWeight: "900" },
  signIn: { marginHorizontal: 16, backgroundColor: colors.gold, borderRadius: 999, paddingVertical: 14, alignItems: "center" },
  signInText: { color: colors.onGold, fontWeight: "800" },
});

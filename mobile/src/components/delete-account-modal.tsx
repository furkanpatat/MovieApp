import { useState } from "react";
import { ActivityIndicator, KeyboardAvoidingView, Modal, Platform, Pressable, StyleSheet, Text, TextInput, View } from "react-native";

import { useT } from "@/i18n";
import { AccountDeletionError, deleteAccount } from "@/lib/session";
import { colors, radius } from "@/theme";

/**
 * Delete account (the stores require it in the app): confirmed with the
 * password. On success the session is gone and Profile shows signed out.
 */
export function DeleteAccountModal({ visible, onClose }: { visible: boolean; onClose: () => void }) {
  const { t } = useT();
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    if (busy) return;
    setPassword("");
    setError(null);
    onClose();
  };

  const submit = async () => {
    if (!password || busy) return;
    setBusy(true);
    setError(null);
    try {
      await deleteAccount(password);
      setPassword("");
      onClose();
    } catch (e) {
      setError(e instanceof AccountDeletionError && e.wrongPassword ? t.profile.deleteWrongPassword : t.profile.deleteFailed);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={close} statusBarTranslucent>
      <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={styles.overlay}>
        <Pressable style={StyleSheet.absoluteFill} onPress={close} accessibilityLabel={t.discover.close} />
        <View style={styles.card}>
          <Text style={styles.title}>{t.profile.deleteAccount}</Text>
          <Text style={styles.text}>{t.profile.deleteWarning}</Text>
          <Text style={styles.label}>{t.profile.deleteConfirm}</Text>
          <TextInput
            style={styles.input}
            placeholder={t.login.password}
            placeholderTextColor={colors.dim}
            secureTextEntry
            textContentType="password"
            autoComplete="current-password"
            value={password}
            onChangeText={setPassword}
            onSubmitEditing={() => void submit()}
            editable={!busy}
          />
          {error && <Text style={styles.error}>{error}</Text>}
          <View style={styles.row}>
            <Pressable style={({ pressed }) => [styles.cancel, pressed && { opacity: 0.8 }]} onPress={close} disabled={busy}>
              <Text style={styles.cancelText}>{t.profile.cancel}</Text>
            </Pressable>
            <Pressable
              style={({ pressed }) => [styles.delete, (!password || busy) && { opacity: 0.5 }, pressed && { opacity: 0.8 }]}
              onPress={() => void submit()}
              disabled={!password || busy}
            >
              {busy ? <ActivityIndicator color="#fff" /> : <Text style={styles.deleteText}>{t.profile.deleteForever}</Text>}
            </Pressable>
          </View>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  overlay: { flex: 1, backgroundColor: "rgba(0,0,0,0.7)", justifyContent: "center", alignItems: "center", padding: 24 },
  card: { backgroundColor: colors.card, borderRadius: radius.lg, padding: 22, width: "100%", maxWidth: 440, borderWidth: 1, borderColor: colors.border },
  title: { color: colors.danger, fontSize: 20, fontWeight: "800", marginBottom: 10 },
  text: { color: colors.mute, lineHeight: 20, marginBottom: 18 },
  label: { color: colors.text, fontWeight: "600", marginBottom: 8 },
  input: { backgroundColor: colors.bg, borderWidth: 1, borderColor: colors.border, color: colors.text, paddingHorizontal: 14, paddingVertical: 12, borderRadius: radius.md, fontSize: 16 },
  error: { color: colors.danger, marginTop: 8 },
  row: { flexDirection: "row", gap: 10, marginTop: 20 },
  cancel: { flex: 1, paddingVertical: 12, alignItems: "center", borderRadius: 999, backgroundColor: colors.cardHi },
  cancelText: { color: colors.text, fontWeight: "700" },
  delete: { flex: 1, paddingVertical: 12, alignItems: "center", borderRadius: 999, backgroundColor: colors.danger },
  deleteText: { color: "#fff", fontWeight: "800" },
});

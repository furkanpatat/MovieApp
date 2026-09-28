import { router } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, KeyboardAvoidingView, Pressable, StyleSheet, Text, TextInput, View } from "react-native";

import { ApiError } from "@/lib/api";
import { useT } from "@/i18n";
import { useSignIn } from "@/lib/queries";
import { colors, radius } from "@/theme";

/** Sign in or create an account (the same account as the web app). */
export default function Login() {
  const [register, setRegister] = useState(false);
  const [login, setLogin] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const m = useSignIn();
  const { t } = useT();

  const submit = () =>
    m.mutate(
      { login: login.trim(), password, register: register ? { username: login.trim(), email: email.trim() } : undefined },
      { onSuccess: () => router.back() },
    );

  const error =
    m.error instanceof ApiError
      ? m.error.status === 401
        ? t.login.wrong
        : m.error.status === 429
          ? t.login.tooMany
          : m.error.message
      : m.error
        ? t.common.unreachable
        : null;

  return (
    <KeyboardAvoidingView behavior="padding" style={styles.screen}>
      <Text style={styles.heading}>{register ? t.login.create : t.login.welcome}</Text>
      <View style={styles.form}>
        <TextInput
          style={styles.input}
          placeholder={register ? t.login.username : t.login.usernameOrEmail}
          placeholderTextColor={colors.dim}
          autoCapitalize="none"
          autoCorrect={false}
          textContentType="username"
          value={login}
          onChangeText={setLogin}
        />
        {register && (
          <TextInput
            style={styles.input}
            placeholder={t.login.email}
            placeholderTextColor={colors.dim}
            autoCapitalize="none"
            keyboardType="email-address"
            textContentType="emailAddress"
            value={email}
            onChangeText={setEmail}
          />
        )}
        <TextInput
          style={styles.input}
          placeholder={t.login.password}
          placeholderTextColor={colors.dim}
          secureTextEntry
          textContentType={register ? "newPassword" : "password"}
          value={password}
          onChangeText={setPassword}
          onSubmitEditing={submit}
        />
        {error && <Text style={styles.error}>{error}</Text>}
        <Pressable onPress={submit} disabled={m.isPending} style={({ pressed }) => [styles.primary, (pressed || m.isPending) && { opacity: 0.8 }]}>
          {m.isPending ? <ActivityIndicator color={colors.onGold} /> : <Text style={styles.primaryText}>{register ? t.login.signUp : t.common.signIn}</Text>}
        </Pressable>
        <Pressable onPress={() => (setRegister((r) => !r), m.reset())} hitSlop={8}>
          <Text style={styles.switch}>{register ? t.login.toSignIn : t.login.toSignUp}</Text>
        </Pressable>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg, padding: 24, paddingTop: 36 },
  heading: { color: colors.text, fontSize: 28, fontWeight: "800" },
  form: { marginTop: 20, gap: 12 },
  input: { backgroundColor: colors.card, color: colors.text, borderRadius: radius.md, paddingHorizontal: 16, paddingVertical: 14, fontSize: 16, borderWidth: 1, borderColor: colors.border },
  error: { color: colors.danger },
  primary: { backgroundColor: colors.gold, borderRadius: 999, paddingVertical: 14, alignItems: "center", marginTop: 4 },
  primaryText: { color: colors.onGold, fontWeight: "800", fontSize: 16 },
  switch: { color: colors.gold, textAlign: "center", marginTop: 8, fontWeight: "600" },
});

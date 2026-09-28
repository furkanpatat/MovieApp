import { StyleSheet, Text, View } from "react-native";

import { colors } from "@/theme";

/** The vertical trailer feed: the next milestone. */
export default function Discover() {
  return (
    <View style={styles.screen}>
      <Text style={styles.title}>Discover</Text>
      <Text style={styles.sub}>The trailer feed is coming in the next update.</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg, alignItems: "center", justifyContent: "center", padding: 32 },
  title: { color: colors.text, fontSize: 28, fontWeight: "800" },
  sub: { color: colors.mute, marginTop: 8, textAlign: "center" },
});

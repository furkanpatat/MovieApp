import * as Haptics from "expo-haptics";
import { Pressable, StyleSheet, Text, View } from "react-native";
import Animated, { useAnimatedStyle, useSharedValue, withSequence, withTiming } from "react-native-reanimated";

import { useMode } from "@/store/mode";
import { colors } from "@/theme";

/** The brand is the switch: Kino + Cut (movies) or Show (series), with a
 *  flip on the suffix, like the web app's logo. */
export function LogoSwitch() {
  const { mode, toggle } = useMode();
  const flip = useSharedValue(0);
  const style = useAnimatedStyle(() => ({ transform: [{ perspective: 400 }, { rotateX: `${flip.value}deg` }] }));

  const onPress = () => {
    void Haptics.selectionAsync();
    flip.value = withSequence(withTiming(90, { duration: 140 }), withTiming(0, { duration: 180 }));
    setTimeout(toggle, 140);
  };

  return (
    <Pressable onPress={onPress} accessibilityRole="switch" accessibilityState={{ checked: mode === "tv" }} hitSlop={12}>
      <View style={styles.row}>
        <View style={styles.mark}>
          <Text style={styles.markText}>K</Text>
        </View>
        <Text style={styles.kino}>Kino</Text>
        <Animated.Text style={[styles.suffix, style]}>{mode === "tv" ? "Show" : "Cut"}</Animated.Text>
        <Text style={styles.swap}>⇄</Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", alignItems: "center", gap: 2 },
  mark: { width: 30, height: 30, borderRadius: 9, backgroundColor: colors.gold, alignItems: "center", justifyContent: "center", marginRight: 8 },
  markText: { color: colors.onGold, fontWeight: "900", fontSize: 17 },
  kino: { color: colors.text, fontSize: 22, fontWeight: "800", letterSpacing: -0.5 },
  suffix: { color: colors.gold, fontSize: 22, fontWeight: "800", letterSpacing: -0.5 },
  swap: { color: colors.dim, fontSize: 14, marginLeft: 6 },
});

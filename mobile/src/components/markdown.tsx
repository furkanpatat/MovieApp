import { Fragment } from "react";
import { StyleSheet, Text, View } from "react-native";

import { colors } from "@/theme";

/**
 * The little Markdown the concierge writes: paragraphs, "- " bullets,
 * **bold** and *italic*. Anything else shows as plain text.
 */
export function Markdown({ text, color = colors.text }: { text: string; color?: string }) {
  const blocks = text.trim().split(/\n{2,}/);
  return (
    <View style={{ gap: 8 }}>
      {blocks.map((block, i) => {
        const lines = block.split("\n");
        const bullets = lines.every((l) => /^\s*[-*•]\s+/.test(l));
        if (bullets) {
          return (
            <View key={i} style={{ gap: 4 }}>
              {lines.map((l, j) => (
                <View key={j} style={styles.bullet}>
                  <Text style={[styles.text, { color: colors.gold }]}>•</Text>
                  <Text style={[styles.text, { color, flex: 1 }]}>{inline(l.replace(/^\s*[-*•]\s+/, ""))}</Text>
                </View>
              ))}
            </View>
          );
        }
        return (
          <Text key={i} style={[styles.text, { color }]}>
            {lines.map((l, j) => (
              <Fragment key={j}>
                {j > 0 && "\n"}
                {inline(l)}
              </Fragment>
            ))}
          </Text>
        );
      })}
    </View>
  );
}

/** **bold** and *italic* spans within a line. */
function inline(line: string) {
  return line.split(/(\*\*[^*]+\*\*|\*[^*]+\*)/g).map((part, i) => {
    if (/^\*\*[^*]+\*\*$/.test(part)) return <Text key={i} style={styles.bold}>{part.slice(2, -2)}</Text>;
    if (/^\*[^*]+\*$/.test(part)) return <Text key={i} style={styles.italic}>{part.slice(1, -1)}</Text>;
    return part;
  });
}

const styles = StyleSheet.create({
  text: { fontSize: 15, lineHeight: 21 },
  bold: { fontWeight: "700" },
  italic: { fontStyle: "italic", color: colors.mute },
  bullet: { flexDirection: "row", gap: 8 },
});

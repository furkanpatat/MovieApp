import { NativeTabs } from "expo-router/unstable-native-tabs";
import { Platform } from "react-native";

import { useT } from "@/i18n";
import { colors } from "@/theme";

/** The five tabs, on the platform's own tab bar (Liquid Glass on iOS).
 *  Every screen lays out its own insets (Home's hero and Discover's feed
 *  run under the status bar), so the automatic ScrollView insets are off. */
export default function TabLayout() {
  const { t } = useT();
  return (
    <NativeTabs
      tintColor={colors.gold}
      iconColor={{ default: colors.mute, selected: colors.gold }}
      // Android's Material bar, in the app's colors (iOS keeps its glass):
      // the dark background, a soft gold pill, every label shown.
      {...(Platform.OS === "android" && {
        backgroundColor: colors.bg,
        indicatorColor: "rgba(251,191,36,0.16)",
        rippleColor: "rgba(251,191,36,0.12)",
        labelVisibilityMode: "labeled" as const,
        labelStyle: { default: { color: colors.mute }, selected: { color: colors.gold } },
      })}
    >
      <NativeTabs.Trigger name="index" disableAutomaticContentInsets>
        <NativeTabs.Trigger.Label>{t.tabs.home}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "house", selected: "house.fill" }} md={{ default: "home", selected: "home_filled" }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="discover" disableAutomaticContentInsets>
        <NativeTabs.Trigger.Label>{t.tabs.discover}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "play.rectangle", selected: "play.rectangle.fill" }} md={{ default: "smart_display", selected: "smart_display" }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="concierge" disableAutomaticContentInsets>
        <NativeTabs.Trigger.Label>{t.tabs.ask}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf="sparkles" md="auto_awesome" />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="search" disableAutomaticContentInsets>
        <NativeTabs.Trigger.Label>{t.tabs.search}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf="magnifyingglass" md="search" />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="profile" disableAutomaticContentInsets>
        <NativeTabs.Trigger.Label>{t.tabs.profile}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "person.crop.circle", selected: "person.crop.circle.fill" }} md={{ default: "account_circle", selected: "account_circle" }} />
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}

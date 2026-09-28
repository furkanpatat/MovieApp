import { NativeTabs } from "expo-router/unstable-native-tabs";

import { colors } from "@/theme";

/** The four tabs, on the platform's own tab bar (Liquid Glass on iOS).
 *  Home and Search lay out their own insets (Home's hero runs under the
 *  status bar), so the automatic ScrollView insets are off there. */
export default function TabLayout() {
  return (
    <NativeTabs tintColor={colors.gold} iconColor={{ default: colors.mute, selected: colors.gold }}>
      <NativeTabs.Trigger name="index" disableAutomaticContentInsets>
        <NativeTabs.Trigger.Label>Home</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "house", selected: "house.fill" }} md={{ default: "home", selected: "home_filled" }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="discover">
        <NativeTabs.Trigger.Label>Discover</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "play.rectangle", selected: "play.rectangle.fill" }} md={{ default: "smart_display", selected: "smart_display" }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="search" disableAutomaticContentInsets>
        <NativeTabs.Trigger.Label>Search</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf="magnifyingglass" md="search" />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="profile">
        <NativeTabs.Trigger.Label>Profile</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "person.crop.circle", selected: "person.crop.circle.fill" }} md={{ default: "account_circle", selected: "account_circle" }} />
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}

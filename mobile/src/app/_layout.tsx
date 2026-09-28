import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DarkTheme, Stack, ThemeProvider } from "expo-router";
import * as ScreenOrientation from "expo-screen-orientation";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useEffect, useState } from "react";
import { useWindowDimensions } from "react-native";

import { useLocale, useT } from "@/i18n";
import { TABLET_MIN_SIDE } from "@/lib/layout";
import { useAuth } from "@/store/auth";
import { colors } from "@/theme";

void SplashScreen.preventAutoHideAsync();

const theme = { ...DarkTheme, colors: { ...DarkTheme.colors, background: colors.bg, card: colors.bg, primary: colors.gold, text: colors.text } };

/** Root: a Stack over the tabs, so a title or the sign-in sheet opens on
 *  top of whichever tab you're on. */
export default function RootLayout() {
  const [client] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: 1 } } }));
  const ready = useAuth((s) => s.ready);
  const { t } = useT();
  const { width, height } = useWindowDimensions();
  const tablet = Math.min(width, height) >= TABLET_MIN_SIDE;

  // Phones stay upright (the feed and the hero are built for it); tablets
  // turn freely, and every screen lays itself out for the window.
  useEffect(() => {
    void (tablet ? ScreenOrientation.unlockAsync() : ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.PORTRAIT_UP)).catch(() => {});
  }, [tablet]);

  useEffect(() => {
    void useAuth.getState().restore();
    void useLocale.getState().restore();
  }, []);
  useEffect(() => {
    if (ready) void SplashScreen.hideAsync();
  }, [ready]);

  return (
    <QueryClientProvider client={client}>
      <ThemeProvider value={theme}>
        <StatusBar style="light" />
        <Stack screenOptions={{ contentStyle: { backgroundColor: colors.bg } }}>
          <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
          <Stack.Screen
            name="title/[media]/[id]"
            options={{ headerTransparent: true, headerTitle: "", headerBackButtonDisplayMode: "minimal", headerTintColor: colors.text }}
          />
          <Stack.Screen
            name="party/[id]"
            options={{ headerTitle: t.party.header, headerBackButtonDisplayMode: "minimal", headerTintColor: colors.text, headerStyle: { backgroundColor: colors.bg } }}
          />
          <Stack.Screen name="login" options={{ presentation: "formSheet", headerShown: false, sheetGrabberVisible: true, sheetAllowedDetents: [0.75] }} />
        </Stack>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

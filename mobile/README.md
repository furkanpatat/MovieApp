# KinoCut mobile

The KinoCut ⇄ KinoShow app for iOS and Android: Expo (SDK 57), React Native,
Expo Router with native tabs. It talks to the same Go API as the web app,
authenticating with `Authorization: Bearer` (the session token is kept in the
Keychain / Keystore via `expo-secure-store`).

## Run

```bash
npm install
npx expo start --ios        # or --android, or scan the QR code with Expo Go
```

By default the app uses the live API (`https://api.kinora.duckdns.org`). To
point it at the local stack (`docker compose up -d` in the repo root), copy
`.env.example` to `.env.local` and set `EXPO_PUBLIC_API_URL`:

- iOS simulator: `http://localhost:8000`
- a phone on your Wi-Fi: `http://<your Mac's LAN IP>:8000`

## Android APK (install on any Android phone)

```bash
export ANDROID_HOME=~/Library/Android/sdk JAVA_HOME=$(/usr/libexec/java_home -v 21)
npx expo prebuild --platform android      # generates android/ (not committed)
cd android && ./gradlew assembleRelease    # -> app/build/outputs/apk/release/app-release.apk
```

The APK is signed with the debug key: fine to sideload (send it to the
phone, open it, allow installing from that app). The Play Store needs a
release keystore (or `npx eas-cli@latest build -p android`).

## Check

```bash
npx expo lint
npx tsc --noEmit
```

## Layout

```
src/app/            screens (Expo Router): (tabs)/ Home, Discover, Search, Profile;
                    title/[media]/[id] details; login sheet
src/components/     logo switch, poster card, title row
src/lib/            API client, react-query hooks, TMDB image URLs
src/store/          session (secure store), movies/series mode
src/types/          API types, shared with web/src/types
```

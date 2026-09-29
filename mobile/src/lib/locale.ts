import { getLocales } from "expo-localization";

/** The phone's language, as the API understands it (Turkish or English). */
export function deviceLocale(): "en" | "tr" {
  return getLocales()[0]?.languageCode === "tr" ? "tr" : "en";
}

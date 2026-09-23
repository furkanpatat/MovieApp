import { useCallback, useSyncExternalStore } from "react";

/**
 * Live result of a CSS media query. False during SSR and the hydration pass
 * (there is no viewport on the server), then the real value.
 */
export function useMediaQuery(query: string) {
  const subscribe = useCallback(
    (onChange: () => void) => {
      const mql = window.matchMedia(query);
      mql.addEventListener("change", onChange);
      return () => mql.removeEventListener("change", onChange);
    },
    [query],
  );

  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(query).matches,
    () => false,
  );
}

/**
 * Which route the previous history entry is: closing a modal goes back at
 * once when it's another modal (a person over a title, a title over a
 * person), and plays its exit animation only when it's returning to a page.
 * The browser's Navigation API knows the entries; where it's missing, the
 * app's own record of route changes stands in (NavTracker, in Providers).
 */

/** Routes that open in the @modal slot over the current page. */
const MODAL_ROUTE = /^\/(movies|tv|person)\/[^/]+\/?$/;

const stack: string[] = [];
let popping = false;
if (typeof window !== "undefined") {
  window.addEventListener("popstate", () => {
    popping = true;
  });
}

/** Records a route change (from usePathname). */
export function recordPath(path: string) {
  if (stack.at(-1) === path) return;
  if (popping) {
    popping = false;
    if (stack.at(-2) === path) {
      stack.pop(); // Back
      return;
    }
  }
  stack.push(path);
  if (stack.length > 100) stack.shift();
}

interface NavigationLike {
  currentEntry?: { index: number } | null;
  entries?: () => { url: string | null }[];
}

/** The previous history entry's path, or null when unknown. */
export function previousPath(): string | null {
  const nav = (window as unknown as { navigation?: NavigationLike }).navigation;
  if (nav?.currentEntry && nav.entries) {
    const prev = nav.entries()[nav.currentEntry.index - 1];
    return prev?.url ? new URL(prev.url).pathname : null;
  }
  return stack.at(-2) ?? null;
}

export function isModalRoute(path: string | null): boolean {
  return !!path && MODAL_ROUTE.test(path);
}

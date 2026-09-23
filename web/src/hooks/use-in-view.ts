import { useEffect, useRef, useState } from "react";

/**
 * True while the returned ref's element is on screen. Used to trigger
 * infinite scroll: cheaper than a scroll listener (IntersectionObserver runs
 * off the main thread's scroll handler entirely) and naturally debounced.
 */
export function useInView<T extends Element>(options?: IntersectionObserverInit) {
  const ref = useRef<T | null>(null);
  const [inView, setInView] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new IntersectionObserver(([entry]) => {
      setInView(entry.isIntersecting);
    }, options);
    observer.observe(el);
    return () => observer.disconnect();
    // Mount-once: the sentinel element itself never swaps out, so there's
    // nothing to re-subscribe to. `options` is typically an inline object
    // literal at call sites and would otherwise re-run this every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { ref, inView };
}

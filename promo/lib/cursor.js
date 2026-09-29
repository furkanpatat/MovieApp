// Injected into every recorded page (addInitScript): a macOS-style pointer
// that follows the (synthetic) mouse, a ripple on each click, and no Next.js
// dev badge. Headless Chrome draws no cursor of its own. On a touch screen
// (the phone) there is no pointer, only a fingertip ripple on each tap.
(() => {
  if (window.top !== window) return;
  const css = `
    nextjs-portal { display: none !important; }
    #__promo_cursor { position: fixed; left: 0; top: 0; z-index: 2147483647; pointer-events: none;
      width: 30px; height: 30px; will-change: transform; filter: drop-shadow(0 3px 8px rgba(0,0,0,.55)); }
    .__promo_ripple { position: fixed; z-index: 2147483646; pointer-events: none; width: 44px; height: 44px;
      margin: -22px 0 0 -22px; border-radius: 50%; border: 2px solid rgba(251,191,36,.95);
      background: rgba(251,191,36,.18); animation: __promo_ripple .55s ease-out forwards; }
    @keyframes __promo_ripple { from { transform: scale(.35); opacity: 1 } to { transform: scale(1.35); opacity: 0 } }`;
  const install = () => {
    if (document.getElementById("__promo_cursor")) return;
    const style = document.createElement("style");
    style.textContent = css;
    document.documentElement.appendChild(style);
    const c = document.createElement("div");
    c.id = "__promo_cursor";
    c.innerHTML = '<svg width="30" height="30" viewBox="0 0 30 30"><path d="M6 3.5 L6 24 L11.2 19.2 L14.8 27.2 L18.6 25.6 L15.1 17.7 L22 17.7 Z" fill="#fff" stroke="#09090b" stroke-width="1.7" stroke-linejoin="round"/></svg>';
    const saved = JSON.parse(sessionStorage.getItem("__promo_pos") || "null") || { x: -100, y: -100 };
    const place = (x, y) => { c.style.transform = `translate(${x - 6}px, ${y - 3.5}px)`; };
    place(saved.x, saved.y);
    if (!matchMedia("(pointer: coarse)").matches) document.documentElement.appendChild(c);
    addEventListener("mousemove", (e) => { place(e.clientX, e.clientY); sessionStorage.setItem("__promo_pos", JSON.stringify({ x: e.clientX, y: e.clientY })); }, true);
    addEventListener("mousedown", (e) => {
      const r = document.createElement("div");
      r.className = "__promo_ripple";
      r.style.left = e.clientX + "px";
      r.style.top = e.clientY + "px";
      document.documentElement.appendChild(r);
      setTimeout(() => r.remove(), 600);
    }, true);
  };
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", install);
  else install();
})();

import { isFolded } from "./prefs";

// Hints (`[data-tip]`): what a control is FOR, beside it, on hover and on focus.
// Ported from ui.js. Drawn outside the sidebar because it scrolls and would clip
// them. The sidebar's links carry data-name too: folded to icons, each is NAMED;
// open, a hint would repeat the label, so none is drawn (except the update icon
// and the fold button, which have no label to repeat).

let hint: HTMLElement | null = null;
let hinted: Element | null = null;

function hide() {
  if (hint) hint.style.display = "none";
  hinted?.removeAttribute("aria-describedby");
  hinted = null;
}

function text(el: HTMLElement) {
  if (isFolded() && el.dataset.name && el.closest(".app-sidebar")) return el.dataset.name;
  return el.getAttribute("data-tip") ?? "";
}

function show(target: HTMLElement) {
  if (!hint) {
    hint = document.createElement("div");
    hint.id = "ui-hint";
    hint.className = "ui-hint";
    hint.setAttribute("role", "tooltip");
    document.body.appendChild(hint);
  }
  hide();
  hint.textContent = text(target);
  hint.style.display = "block";
  const r = target.getBoundingClientRect();
  let left = r.right + 8;
  if (left + hint.offsetWidth > window.innerWidth - 8) left = r.left - hint.offsetWidth - 8;
  hint.style.left = `${Math.max(8, left)}px`;
  hint.style.top = `${r.top + r.height / 2 - hint.offsetHeight / 2}px`;
  target.setAttribute("aria-describedby", "ui-hint");
  hinted = target;
}

function targetOf(e: Event): HTMLElement | null {
  const t = e.target;
  if (!(t instanceof Element)) return null;
  const el = t.closest<HTMLElement>("[data-tip], [data-name]");
  if (!el) return null;
  const free = !el.closest(".app-sidebar") || el.hasAttribute("data-update-icon") || el.hasAttribute("data-nav-fold");
  if (free) return el.hasAttribute("data-tip") ? el : null;
  return isFolded() ? el : null;
}

export function installHints(): () => void {
  const over = (e: Event) => {
    const t = targetOf(e);
    if (t && t !== hinted) show(t);
    else if (!t && hinted && !hinted.matches(":focus-visible")) hide();
  };
  const focusin = (e: Event) => {
    const t = targetOf(e);
    if (t?.matches(":focus-visible")) show(t);
  };
  const key = (e: KeyboardEvent) => e.key === "Escape" && hide();
  document.addEventListener("mouseover", over);
  document.addEventListener("focusin", focusin);
  document.addEventListener("focusout", hide);
  document.addEventListener("scroll", hide, { passive: true, capture: true });
  document.addEventListener("keydown", key);
  document.addEventListener("mouseleave", hide);
  return () => {
    document.removeEventListener("mouseover", over);
    document.removeEventListener("focusin", focusin);
    document.removeEventListener("focusout", hide);
    document.removeEventListener("scroll", hide, { capture: true });
    document.removeEventListener("keydown", key);
    document.removeEventListener("mouseleave", hide);
    hide();
  };
}

export const hideHint = hide;

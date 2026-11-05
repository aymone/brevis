// components.ButtonClass: a button's look as a class string, for a <button>, a
// link or a form's submit alike. Written out whole so the sheet has each class.
const base =
  "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-card font-medium whitespace-nowrap transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-strong disabled:pointer-events-none disabled:opacity-50";

export function buttonClass(kind: "secondary" | "primary" | "danger" = "secondary", size: "regular" | "small" = "regular"): string {
  const dim = size === "small" ? "h-[26px] px-2.5 text-[0.78rem]" : "h-8 px-3 text-[0.82rem]";
  const look =
    kind === "primary"
      ? "bg-accent-strong text-on-accent hover:bg-accent-strong/90"
      : kind === "danger"
        ? "border border-line bg-surface text-state-failed hover:bg-state-failed/10"
        : "border border-line bg-surface text-ink hover:bg-surface-2";
  return `${base} ${dim} ${look}`;
}

// The Runs list's state control (the one list not on ui/filters yet).
export const segGroup = "inline-flex h-8 shrink-0 items-center gap-0.5 rounded-card border border-line bg-surface p-0.5";
export const segOn = "inline-flex h-full shrink-0 items-center gap-1.5 rounded-[2px] bg-surface-2 px-2.5 text-[0.8rem] font-medium whitespace-nowrap text-ink";
export const segOff = "inline-flex h-full shrink-0 items-center gap-1.5 rounded-[2px] px-2.5 text-[0.8rem] whitespace-nowrap text-muted transition-colors duration-150 hover:text-ink";

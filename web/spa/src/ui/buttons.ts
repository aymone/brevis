// The console's button looks (web/components/controls.go), as the class
// strings the server's sheet already compiles.
const base =
  "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-card font-medium whitespace-nowrap transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-strong disabled:pointer-events-none disabled:opacity-50";

const kinds = {
  secondary: "border border-line bg-surface text-ink hover:bg-surface-2",
  primary: "bg-accent-strong text-on-accent hover:bg-accent-strong/90",
  danger: "border border-line bg-surface text-state-failed hover:bg-state-failed/10",
};
const sizes = { regular: "h-8 px-3 text-[0.82rem]", small: "h-[26px] px-2.5 text-[0.78rem]" };

export const buttonClass = (kind: keyof typeof kinds = "secondary", size: keyof typeof sizes = "regular") => `${base} ${sizes[size]} ${kinds[kind]}`;

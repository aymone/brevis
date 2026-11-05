// The server's own marks (web/components/logos/*.svg), read as text so a mark is
// drawn inline and takes the theme's colors. A chunk of its own: logo.tsx loads
// it after the first paint and the tiles stand in until it arrives.
const files = import.meta.glob("../../../components/logos/*.svg", { query: "?raw", import: "default", eager: true }) as Record<string, string>;

export const marks: Record<string, string> = Object.fromEntries(
  Object.entries(files).map(([path, svg]) => [path.replace(/^.*\/(\w+)\.svg$/, "$1"), svg.trim()]),
);

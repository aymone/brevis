import { glyphs } from "./glyphs";

type Props = { name: string; className?: string };

/**
 * One glyph, inline, in the text's own colour and at the size the class gives
 * it (`size-4`, `h-4 w-4`). shrink-0 is always added: an icon beside a long
 * label in a flex row is the first thing the browser squeezes. A name nobody
 * drew renders nothing, as on the server.
 */
export function Icon({ name, className = "" }: Props) {
  const body = glyphs[name];
  if (!body) return null;
  return (
    <svg
      className={`${className} shrink-0`.trim()}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      dangerouslySetInnerHTML={{ __html: body }}
    />
  );
}

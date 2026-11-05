import type { CSSProperties, ReactNode } from "react";

type Props = { w?: string | number; h?: string | number; className?: string };

// Skeleton is the one placeholder primitive. Every screen's loading state is
// built from it, with the same grid as the real screen, so nothing jumps when
// the data arrives. aria-hidden: the region that loads says so once, with
// aria-busy, instead of every block announcing itself.
export function Skeleton({ w = "100%", h = "1rem", className = "" }: Props) {
  const style: CSSProperties = { width: w, height: h };
  return <span aria-hidden="true" className={`sk ${className}`} style={style} />;
}

export function SkeletonText({ lines = 3 }: { lines?: number }) {
  return (
    <span className="sk-text" aria-hidden="true">
      {Array.from({ length: lines }, (_, i) => (
        <Skeleton key={i} w={i === lines - 1 ? "60%" : "100%"} />
      ))}
    </span>
  );
}

/** The class of a block that renders the loaded markup with placeholder values (see .sk-real in styles.css). */
export const SK_REAL = "sk-real";

/**
 * A placeholder that takes the place of a loaded value INSIDE the real markup:
 * render the loaded element as usual and put a Ghost where the data goes, with
 * a placeholder about as wide as the data. The line box is the real one, so the
 * height is right by construction (a fixed-height Skeleton is a guess).
 */
export function Ghost({ children = "0000" }: { children?: ReactNode }) {
  return (
    <span aria-hidden="true" className="sk-inline">
      {children}
    </span>
  );
}

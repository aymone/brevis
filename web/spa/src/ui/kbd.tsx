/** A key or a chord, as it is printed on the keyboard (web/components Kbd). */
export function Kbd({ keys }: { keys: string }) {
  return (
    <kbd className="inline-flex h-5 min-w-5 items-center justify-center rounded-card border border-line bg-surface-2 px-1 font-mono text-[0.68rem] leading-none text-muted">{keys}</kbd>
  );
}

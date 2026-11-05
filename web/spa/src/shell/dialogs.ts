import { useSyncExternalStore } from "react";

// Which of the shell's native <dialog>s is open: the palette, the shortcut
// sheet or the release notes. One at a time.
export type DialogName = "palette" | "shortcuts" | "version";
let open: DialogName | null = null;
const subs = new Set<() => void>();

export function openDialog(name: DialogName | null) {
  open = name;
  subs.forEach((s) => s());
}
export const useOpenDialog = () =>
  useSyncExternalStore(
    (s) => (subs.add(s), () => void subs.delete(s)),
    () => open,
  );

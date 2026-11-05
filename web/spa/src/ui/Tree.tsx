import { useRef, useState, type KeyboardEvent, type ReactNode } from "react";

export type TreeItem = {
  id: string;
  parent?: string;
  level: number;
  label: ReactNode;
  expandable?: boolean;
  expanded?: boolean;
};

type Props = {
  items: TreeItem[];
  label: string;
  onToggle: (id: string) => void;
  onActivate: (id: string) => void;
};

// A flat, keyboard-operable tree (WAI-ARIA treeview): the caller says which
// rows are visible, in order, with their level; this owns focus and keys.
// Up/Down/Home/End move, Right opens or enters, Left closes or goes to the
// parent, Enter/Space activates.
export function Tree({ items, label, onToggle, onActivate }: Props) {
  const [focus, setFocus] = useState("");
  const root = useRef<HTMLUListElement>(null);
  const current = items.find((i) => i.id === focus) ?? items[0];

  const go = (id: string | undefined) => {
    if (id === undefined) return;
    setFocus(id);
    root.current?.querySelector<HTMLElement>(`[data-id="${CSS.escape(id)}"]`)?.focus();
  };

  const onKey = (e: KeyboardEvent<HTMLUListElement>) => {
    if (!current) return;
    const at = items.indexOf(current);
    switch (e.key) {
      case "ArrowDown": go(items[at + 1]?.id); break;
      case "ArrowUp": go(items[at - 1]?.id); break;
      case "Home": go(items[0]?.id); break;
      case "End": go(items[items.length - 1]?.id); break;
      case "ArrowRight":
        if (current.expandable && !current.expanded) onToggle(current.id);
        else if (current.expanded) go(items[at + 1]?.id);
        break;
      case "ArrowLeft":
        if (current.expanded) onToggle(current.id);
        else go(current.parent);
        break;
      case "Enter":
      case " ": onActivate(current.id); break;
      default: return;
    }
    e.preventDefault();
  };

  return (
    <ul ref={root} role="tree" aria-label={label} className="tree" onKeyDown={onKey}>
      {items.map((it) => (
        <li
          key={it.id}
          role="treeitem"
          data-id={it.id}
          aria-level={it.level}
          aria-expanded={it.expandable ? !!it.expanded : undefined}
          tabIndex={it === current ? 0 : -1}
          style={{ paddingInlineStart: `${(it.level - 1) * 0.9}rem` }}
          onFocus={(e) => e.target === e.currentTarget && setFocus(it.id)}
          onClick={() => {
            setFocus(it.id);
            onActivate(it.id);
          }}
        >
          {it.label}
        </li>
      ))}
    </ul>
  );
}

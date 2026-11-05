import { useRef, type ChangeEvent, type KeyboardEvent } from "react";

type Props = {
  value: string;
  onChange: (v: string) => void;
  onRun?: () => void;
  label: string;
  placeholder?: string;
  id?: string;
};

// A plain labelled textarea with a line-number gutter. No wrapping, so one
// gutter number is one line. Tab indents; Escape then Tab leaves the field, so
// the keyboard is never trapped. Ctrl/Cmd+Enter runs.
export function CodeEditor({ value, onChange, onRun, label, placeholder, id }: Props) {
  const gutter = useRef<HTMLDivElement>(null);
  const releaseTab = useRef(false);
  const lines = value.split("\n").length;

  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
      e.preventDefault();
      onRun?.();
      return;
    }
    if (e.key === "Escape") {
      releaseTab.current = true;
      return;
    }
    if (e.key !== "Tab") {
      releaseTab.current = false;
      return;
    }
    if (releaseTab.current) {
      releaseTab.current = false;
      return;
    }
    e.preventDefault();
    const el = e.currentTarget;
    const { selectionStart: a, selectionEnd: b } = el;
    if (e.shiftKey) {
      // Outdent the line the caret is on by up to two spaces.
      const from = value.lastIndexOf("\n", a - 1) + 1;
      const cut = /^ {1,2}/.exec(value.slice(from))?.[0].length ?? 0;
      if (!cut) return;
      onChange(value.slice(0, from) + value.slice(from + cut));
      requestAnimationFrame(() => el.setSelectionRange(Math.max(from, a - cut), Math.max(from, b - cut)));
      return;
    }
    onChange(value.slice(0, a) + "  " + value.slice(b));
    requestAnimationFrame(() => el.setSelectionRange(a + 2, a + 2));
  };

  return (
    <div className="editor">
      <div ref={gutter} className="editor-gutter" aria-hidden="true">
        {Array.from({ length: lines }, (_, i) => (
          <span key={i}>{i + 1}</span>
        ))}
      </div>
      <textarea
        id={id}
        aria-label={label}
        className="editor-text"
        value={value}
        placeholder={placeholder}
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        wrap="off"
        onChange={(e: ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value)}
        onKeyDown={onKey}
        onScroll={(e) => {
          if (gutter.current) gutter.current.scrollTop = e.currentTarget.scrollTop;
        }}
      />
    </div>
  );
}

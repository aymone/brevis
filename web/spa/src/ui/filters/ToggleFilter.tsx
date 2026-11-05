import "./filters.css";

/**
 * A yes/no filter ("Show legacy") as a pressed-state button, the same control the log's "Wrap" is: a one-option
 * dropdown would open a popover to offer a single checkbox. Pressed = raised and bold (never colour alone).
 * It stays in the row (it does not fold): it is short, and it is its own applied-filter indicator.
 */
export function ToggleFilter({ label, on, onChange }: { label: string; on: boolean; onChange: (on: boolean) => void }) {
  return (
    <button type="button" aria-pressed={on} className="flt-ctl flt-btn" onClick={() => onChange(!on)}>
      {label}
    </button>
  );
}

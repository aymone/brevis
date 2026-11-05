// ToggleSwitch is an on/off control that acts at once (a switch, not a form
// field): the label says what pressing does. .switch is the server stylesheet's.
export function ToggleSwitch({ on, label, busy, onToggle }: { on: boolean; label: string; busy?: boolean; onToggle: () => void }) {
  return (
    <button type="button" role="switch" aria-checked={on} aria-label={label} title={label} disabled={busy} className="switch disabled:opacity-50" onClick={onToggle}>
      <span className="switch-knob" aria-hidden="true" />
    </button>
  );
}

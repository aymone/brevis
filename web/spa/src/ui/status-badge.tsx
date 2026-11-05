import { RetryMark, Status, runKind } from "./status";

type Props = { status: string; label?: string; attempt?: number };

/** A run's or step's state: the mark and its word. attempt (from 0) adds the retry mark. */
export function StatusBadge({ status, label, attempt = 0 }: Props) {
  return (
    <>
      <Status kind={runKind(status)} label={label} />
      <RetryMark attempt={attempt} />
    </>
  );
}

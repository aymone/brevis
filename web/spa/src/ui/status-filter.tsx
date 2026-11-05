import type { ReactNode } from "react";
import { useT } from "../i18n";
import { Segmented, type Chip, type FilterDef } from "./filters";
import { freshnessKind } from "./status";

/*
 * The freshness question of the Catalog and the Loads list, drawn once. Nine states do not fit one segmented
 * control, so it is split by how often people ask: the three questions asked every visit (everything / what
 * needs a person / what is fine) are the segmented control; the rest sit in a "State" dropdown (not "Status": the compact twin of the segmented control already uses that word), with their
 * counts. It is still ONE URL parameter (`status`): choosing in either replaces the other, so the two never
 * disagree. When the value lives in the dropdown no segment is checked (honest: none of the three is on).
 */
const PRIMARY = ["", "attention", "on_time"];
const NONE = "-";

export function useStatusFilter({ statuses, count, pending, mark, status, onChange }: {
  statuses: readonly string[];
  /** The count of one answer: "" is all. */
  count: (key: string) => number;
  pending: boolean;
  mark: (s: string) => ReactNode;
  status: string;
  onChange: (status: string) => void;
}) {
  const { t } = useT();
  const word = (s: string) => t(`state.${freshnessKind(s)}`);
  // The mark's own label would be read before the word that already names it.
  const dot = (s: string) => <span aria-hidden="true" className="inline-flex">{mark(s)}</span>;
  const rare = statuses.filter((s) => !PRIMARY.includes(s));
  const inDropdown = status !== "" && !PRIMARY.includes(status);
  const primary = (
    <Segmented
      label={t("catalog.band")}
      pending={pending}
      value={inDropdown ? NONE : status}
      onChange={onChange}
      items={[
        { key: "", label: t("catalog.status.all"), count: count("") },
        { key: "attention", label: t("catalog.status.attention"), icon: dot("stale"), count: count("attention"), title: t("catalog.tip.attention") },
        { key: "on_time", label: word("on_time"), icon: dot("on_time"), count: count("on_time"), title: t("catalog.tip.on_time") },
      ]}
    />
  );
  const def: FilterDef = {
    id: "status", label: t("workflows.filter.state"), pending,
    options: rare.map((s) => ({ key: s, label: word(s), icon: dot(s), count: count(s) })),
    value: inDropdown ? [status] : [], onChange: (v) => onChange(v[0] ?? ""),
  };
  const chip: Chip[] = inDropdown ? [{ id: "status", group: t("workflows.filter.state"), value: word(status), onRemove: () => onChange("") }] : [];
  return { primary, def, chip };
}

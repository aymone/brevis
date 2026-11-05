import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { ApiError, getJSON } from "../api/client";
import { fieldErrors, triggerRun, type TriggerBody } from "../api/trigger";
import type { WorkflowDetail } from "../api/workflows";
import { useFormat, useT } from "../i18n";
import { toast } from "../shell/toast";
import { buttonClass } from "../ui/button";
import { Icon } from "../ui/icon";
import { Logo, useLogoTitle } from "../ui/logo";
import { LoadError } from "../ui/query-boundary";
import { Skeleton } from "../ui/skeleton";
import { useRefreshRuns, type Rerun } from "./run-now";

type Param = WorkflowDetail["params"][number];

// The server's rule for a `string` with no pattern (internal/domain/workflow/param.go): checked here first so
// the operator is told before posting; the server checks again.
const SAFE = /^[A-Za-z0-9_.:/=,+@-]*$/;

const isList = (p: Param) => p.type.startsWith("list|");
const elemOf = (p: Param) => (isList(p) ? p.type.slice(5) : p.type);
const itemsOf = (v: string) => v.split(",").map((s) => s.trim()).filter(Boolean);
const joined = (v: string) => itemsOf(v).join(",");
// A boolean is only ever true or false: the old select defaulted to false when the default said nothing.
const norm = (p: Param, v: string) => (p.type === "boolean" ? String(v === "true") : isList(p) ? joined(v) : v.trim());

function quote(v: string) {
  return /^[A-Za-z0-9_.:/=,+@-]+$/.test(v) ? v : `'${v.replace(/'/g, "'\\''")}'`;
}

/** Params by group: the ones with none first, under no heading, then each group in the order it first appears. */
function grouped(ps: Param[]): { name: string; params: Param[] }[] {
  const out: { name: string; params: Param[] }[] = [];
  for (const p of ps) {
    let g = out.find((x) => x.name === p.group);
    if (!g) out.push((g = { name: p.group, params: [] }));
    g.params.push(p);
  }
  return out.sort((a, b) => Number(a.name !== "") - Number(b.name !== ""));
}

const input = "h-8 rounded-card border border-line bg-surface px-2.5 text-[0.82rem] text-ink";
const chipsBox = "flex flex-wrap items-center gap-1.5 rounded-xl border border-line bg-parchment-soft px-2 py-1.5";

/** The old free list: removable chips, Enter or comma adds, Backspace removes the last; what is typed counts on submit. */
function Chips({ id, label, value, describedBy, invalid, onChange, draft }: {
  id: string; label: string; value: string; describedBy?: string; invalid: boolean;
  onChange: (v: string) => void; draft: (text: string) => void;
}) {
  const { t } = useT();
  const [text, setText] = useState("");
  const items = itemsOf(value);
  const add = (raw: string) => {
    const next = [...items];
    // A paste of "a,b,c" adds three; a repeated item is refused here, since the engine would refuse it later.
    for (const part of raw.split(",").map((s) => s.trim())) if (part && !next.includes(part)) next.push(part);
    onChange(next.join(","));
    setText("");
    draft("");
  };
  return (
    <div className={chipsBox}>
      {items.map((item, i) => (
        <span key={item} data-chip className="inline-flex items-center gap-1 rounded-lg bg-ink/5 px-2 py-0.5 font-mono text-xs text-ink">
          {item}
          <button type="button" aria-label={`${t("dialog.close_word")} ${item}`} className="text-muted hover:text-ink" onClick={() => onChange(items.filter((_, j) => j !== i).join(","))}>
            ×
          </button>
        </span>
      ))}
      <input
        id={id}
        type="text"
        value={text}
        aria-label={label}
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy}
        placeholder={t("drawer.list.placeholder")}
        className="min-w-[6rem] flex-1 bg-transparent px-1 py-0.5 font-mono text-sm text-ink outline-none placeholder:text-muted/60"
        onChange={(e) => {
          setText(e.target.value);
          draft(e.target.value);
        }}
        onKeyDown={(e) => {
          if ((e.key === "Enter" && !(e.metaKey || e.ctrlKey)) || e.key === ",") {
            // Enter inside a form submits it; an item being added is not a submit.
            e.preventDefault();
            add(text);
          } else if (e.key === "Backspace" && text === "") onChange(items.slice(0, -1).join(","));
        }}
        onBlur={() => text && add(text)}
      />
    </div>
  );
}

function Field({ p, def, value, problem, onChange, draft }: {
  p: Param; def: string; value: string; problem: string; onChange: (v: string) => void; draft: (text: string) => void;
}) {
  const { t } = useT();
  const logoTitle = useLogoTitle();
  const base = useId();
  const id = `${base}-v`;
  const errId = `${base}-e`;
  const changed = norm(p, value) !== norm(p, def);
  const control: ReactNode = (() => {
    const aria = { "aria-invalid": !!problem || undefined, "aria-describedby": problem ? errId : undefined };
    if (p.type === "boolean")
      return (
        <span className="inline-flex items-center gap-2">
          <button id={id} type="button" role="switch" aria-checked={value === "true"} aria-label={p.name} {...aria} className="switch" onClick={() => onChange(value === "true" ? "false" : "true")}>
            <span className="switch-knob" aria-hidden="true" />
          </button>
          <code className="font-mono text-[12px] text-ink-2">{value === "true" ? "true" : "false"}</code>
        </span>
      );
    if (p.type === "integer")
      return (
        <span className="inline-flex items-stretch">
          <button type="button" aria-label="−" className="w-8 rounded-l-card border border-line text-muted hover:bg-hover hover:text-ink" onClick={() => onChange(String((parseInt(value, 10) || 0) - 1))}>−</button>
          <input id={id} type="number" value={value} step="1" inputMode="numeric" {...aria} onChange={(e) => onChange(e.target.value)} className="h-8 w-28 border-y border-line bg-surface px-2.5 text-center font-mono text-[0.82rem] text-ink focus:border-accent-strong focus:outline-none max-[0px]:border-x" />
          <button type="button" aria-label="+" className="w-8 rounded-r-card border border-line text-muted hover:bg-hover hover:text-ink" onClick={() => onChange(String((parseInt(value, 10) || 0) + 1))}>+</button>
        </span>
      );
    if (isList(p) && p.enum.length > 0) {
      const on = itemsOf(value);
      return (
        <div id={id} role="group" aria-label={p.name} {...aria} className="flex flex-wrap gap-x-3 gap-y-1 rounded-xl border border-line bg-parchment-soft px-3 py-2">
          {p.enum.map((v) => (
            <label key={v} className="flex items-center gap-1.5 text-sm text-ink">
              <input type="checkbox" checked={on.includes(v)} className="accent-ink" onChange={(e) => onChange(p.enum.filter((x) => (x === v ? e.target.checked : on.includes(x))).join(","))} />
              <span className="font-mono text-xs">{v}</span>
            </label>
          ))}
        </div>
      );
    }
    if (isList(p)) return <Chips id={id} label={p.name} value={value} describedBy={problem ? errId : undefined} invalid={!!problem} onChange={onChange} draft={draft} />;
    if (p.enum.length > 0)
      return (
        <select id={id} value={value} {...aria} onChange={(e) => onChange(e.target.value)} className={input}>
          {p.enum.map((v) => <option key={v} value={v}>{v}</option>)}
        </select>
      );
    return <input id={id} type="text" value={value} {...aria} onChange={(e) => onChange(e.target.value)} className="h-8 w-full rounded-card border border-line bg-surface px-2.5 font-mono text-[0.82rem] text-ink placeholder:text-faint focus:border-accent-strong focus:outline-none" />;
  })();
  return (
    <div data-param={p.name} className="rounded-card border border-line bg-surface px-3.5 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <label htmlFor={id} className="font-mono text-[13px] font-medium text-ink">{p.name}</label>
        {changed && <span data-param-changed title={t("drawer.changed")} className="h-2 w-2 rounded-full bg-warn" />}
        {p.uses && (
          <span className="inline-flex items-center gap-1 text-[12.5px] text-ink">
            <Logo k={p.uses} size={18} />
            {logoTitle(p.uses)}
          </span>
        )}
        <span className="ml-auto text-xs text-muted">{def !== "" ? t("drawer.default", { value: def }) : t("workflow.params.required")}</span>
      </div>
      {(p.title || p.description) && (
        <p className="mt-1 text-[12.5px] text-muted">
          {p.title && <strong className="font-medium text-ink">{p.title}. </strong>}
          {p.description}
        </p>
      )}
      <div className="mt-2">{control}</div>
      {p.pattern && (
        <p className="mt-1 text-xs text-faint">
          {t("drawer.pattern")} <code className="font-mono text-ink-2">{p.pattern}</code>
        </p>
      )}
      {problem && <p id={errId} data-param-error role="alert" className="mt-1 text-xs text-state-failed">{problem}</p>}
      {p.warn && changed && <p data-param-warn className="mt-1 text-xs text-warn">{p.warn}</p>}
    </div>
  );
}

/** "Now" or "Another slot": the schedule's last slots, each with the window it processes, or a time of one's own. */
function SlotPicker({ d, when, setWhen, sel, setSel, at, setAt, problem }: {
  d: WorkflowDetail; when: "now" | "slot"; setWhen: (w: "now" | "slot") => void; sel: string; setSel: (v: string) => void;
  at: string; setAt: (v: string) => void; problem?: string;
}) {
  const { t } = useT();
  const err = useId();
  return (
    <fieldset className="space-y-2">
      <legend className="mb-1.5 font-medium text-ink">{t("workflows.run_now.slot")}</legend>
      <div className="inline-flex h-8 items-center gap-0.5 rounded-card border border-line bg-surface p-0.5">
        {(["now", "slot"] as const).map((w) => (
          <label key={w} className="run-when">
            <input type="radio" name="when" value={w} checked={when === w} onChange={() => setWhen(w)} className="sr-only" />
            {t(w === "now" ? "workflows.run_now.now" : "workflows.run_now.other")}
          </label>
        ))}
      </div>
      {when === "slot" && (
        <div className="space-y-2">
          <select value={sel} aria-label={t("workflows.run_now.slot")} aria-invalid={!!problem || undefined} aria-describedby={problem ? err : undefined} onChange={(e) => setSel(e.target.value)} className="h-8 w-full rounded-card border border-line bg-surface px-2.5 text-[0.82rem] text-ink">
            {d.slots.map((s) => <option key={s.at} value={s.at}>{s.label}</option>)}
            <option value="other">{t("workflows.run_now.another")}</option>
          </select>
          {sel === "other" && (
            <input type="datetime-local" step="60" required value={at} aria-label={t("workflows.run_now.another")} aria-invalid={!!problem || undefined} aria-describedby={problem ? err : undefined} onChange={(e) => setAt(e.target.value)} className="h-8 w-full rounded-card border border-line bg-surface px-2.5 text-[0.82rem] text-ink" />
          )}
          <p className="text-xs text-muted">{t("workflows.run_now.zone", { zone: d.timezone || "UTC" })}</p>
        </div>
      )}
      {problem && <p id={err} data-slot-error role="alert" className="text-xs text-state-failed">{problem}</p>}
    </fieldset>
  );
}

export default function RunDrawer({ slug, rerun, onClose }: { slug: string; rerun?: Rerun; onClose: () => void }) {
  const { t, n } = useT();
  const { utc } = useFormat();
  const navigate = useNavigate();
  const refresh = useRefreshRuns();
  const ref = useRef<HTMLDialogElement>(null);

  // The same query, and cache, as the workflow's page: the definition is what the form is drawn from.
  const q = useQuery({
    queryKey: ["workflow", slug, ""],
    queryFn: () => getJSON<WorkflowDetail>(`/api/workflows/${encodeURIComponent(slug)}`),
    retry: (count, e) => !(e instanceof ApiError && e.status === 404) && count < 1,
  });
  const d = q.data;

  // A rerun fills each field with what the run used, and a name the definition no longer declares is not sent.
  const params = d?.params ?? [];
  const defOf = (p: Param) => rerun?.params[p.name] ?? p.default;
  const dropped = rerun && d ? Object.keys(rerun.params).filter((k) => !params.some((p) => p.name === k)).sort() : [];

  const [values, setValues] = useState<Record<string, string> | null>(null);
  const [when, setWhen] = useState<"now" | "slot">("now");
  const [sel, setSel] = useState("");
  const [at, setAt] = useState("");
  const [server, setServer] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  // What is typed in a chips field and not yet committed still counts on submit.
  const drafts = useRef<Record<string, string>>({});

  // Initial values wait for the definition; after that the operator's edits are the state.
  useEffect(() => {
    if (d && values === null) {
      setValues(Object.fromEntries(d.params.map((p) => [p.name, norm(p, rerun?.params[p.name] ?? p.default)])));
      setSel(d.slots[0]?.at ?? "other");
    }
  }, [d, values, rerun]);

  useEffect(() => {
    const el = ref.current;
    if (el && !el.open) el.showModal();
    return () => el?.close();
  }, []);

  const value = (p: Param) => values?.[p.name] ?? norm(p, defOf(p));

  const problemOf = (p: Param, v: string): string => {
    if (v === "") return defOf(p) === "" && !isList(p) ? t("drawer.check.required") : "";
    const elem = elemOf(p);
    for (const item of isList(p) ? itemsOf(v) : [v]) {
      if (elem === "integer" && !/^-?\d+$/.test(item)) return t("drawer.check.integer");
      if (elem === "boolean" && item !== "true" && item !== "false") return t("drawer.check.boolean");
      if (p.enum.length && !p.enum.includes(item)) return t("drawer.check.enum");
      if (p.pattern) {
        try {
          if (!new RegExp(p.pattern).test(item)) return t("drawer.check.pattern");
        } catch {
          /* the server says */
        }
      } else if (elem === "string" && !SAFE.test(item)) return t("drawer.check.chars");
    }
    return "";
  };

  const rows = params.map((p) => ({ p, v: value(p) }));
  const problems = new Map(rows.map(({ p, v }) => [p.name, problemOf(p, v) || server[p.name] || ""]));
  const invalid = [...problems.values()].filter(Boolean).length;
  const changed = rows.filter(({ p, v }) => norm(p, v) !== norm(p, defOf(p)));

  const picks = !rerun && !!d && (params.length > 0 ? d.cron !== "" : d.slots.length > 0);
  const slotValue = picks && when === "slot" ? (sel === "other" ? at : sel) : "";
  const slotMissing = picks && when === "slot" && sel === "other" && at === "";

  const post = useMutation({
    mutationFn: (body: TriggerBody) => triggerRun(slug, body),
    onSuccess: (r) => {
      toast(t(rerun ? "overview.rerun_created" : "workflows.run_created", { workflow: slug }), { kind: "run" });
      void refresh(slug);
      onClose();
      navigate(r.href);
    },
    onError: (e) => {
      const f = fieldErrors(e);
      setServer(f);
      setFormError(Object.keys(f).length > 0 && Object.keys(f).every((k) => k === "slot" || problems.has(k)) ? "" : (e as Error).message);
    },
  });

  const submit = () => {
    if (!d || !values || post.isPending) return;
    // Commit what is typed in a chips field before reading the values.
    const final = { ...values };
    for (const p of params) {
      const extra = drafts.current[p.name];
      if (extra) final[p.name] = [...itemsOf(final[p.name] ?? ""), ...itemsOf(extra).filter((x) => !itemsOf(final[p.name] ?? "").includes(x))].join(",");
    }
    if (params.some((p) => problemOf(p, final[p.name] ?? "")) || slotMissing) return;
    const body: TriggerBody = { params: Object.fromEntries(params.map((p) => [p.name, isList(p) ? itemsOf(final[p.name] ?? "") : final[p.name] ?? ""])) };
    if (slotValue) body.slot = slotValue;
    if (rerun) body.from_run = rerun.runId;
    setFormError("");
    post.mutate(body);
  };

  const onKey = (e: KeyboardEvent<HTMLFormElement>) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      submit();
    }
  };

  const edit = (p: Param, v: string) => {
    setValues((cur) => ({ ...(cur ?? {}), [p.name]: v }));
    setServer((s) => (s[p.name] ? { ...s, [p.name]: "" } : s));
  };
  const reset = () => {
    setValues(Object.fromEntries(params.map((p) => [p.name, norm(p, defOf(p))])));
    setServer({});
    drafts.current = {};
  };

  const bare = !rerun && !!d && params.length === 0;
  const title = rerun ? t("drawer.title.rerun") : t("drawer.title.run");
  const payload = {
    params: Object.fromEntries(rows.map(({ p, v }) => [p.name, v])),
    ...(slotValue ? { slot: slotValue } : {}),
  };
  const given = changed.map(({ p, v }) => [p.name, v] as const);
  const cli = slotValue
    ? [`curl -X POST ${location.origin}/workflows/${slug}/trigger`, `-d slot=${quote(slotValue)}`, ...given.map(([k, v]) => `-d ${quote(`param.${k}=${v}`)}`)].join(" \\\n  ")
    : [`brevis run ${slug}`, ...(d?.project ? [`--project ${quote(d.project)}`] : []), ...given.map(([k, v]) => `--param ${quote(`${k}=${v}`)}`)].join(" ");

  const status = invalid
    ? n("drawer.status.invalid", invalid, { n: invalid })
    : changed.length
      ? `● ${n("drawer.status.changed", changed.length, { n: changed.length })}`
      : t("drawer.status.defaults");

  const head = bare ? (
    <header className="dialog-head">
      <h2 id="run-title" className="text-[0.95rem] font-semibold text-ink">{t("workflows.run_now.title", { workflow: slug })}</h2>
      <button type="button" aria-label={t("dialog.close")} onClick={onClose} className="flex h-8 w-8 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink">
        <Icon name="x" className="h-4 w-4" />
      </button>
    </header>
  ) : (
    <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
      <div className="min-w-0">
        <p className="text-[11px] font-medium tracking-[0.08em] text-muted uppercase">{title}</p>
        <p id="run-title" className="mt-0.5 truncate font-mono text-[1rem] font-medium text-ink">{slug}</p>
        {!rerun && <p className="mt-1 text-[12.5px] text-muted">{t("drawer.lead")}</p>}
      </div>
      <button type="button" aria-label={t("dialog.close")} onClick={onClose} className="flex h-8 w-8 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink">
        <Icon name="x" className="h-4 w-4" />
      </button>
    </div>
  );

  let body: ReactNode;
  if (q.isError) body = <LoadError onRetry={() => void q.refetch()} />;
  else if (!d || !values)
    body = (
      <div aria-busy="true" className="space-y-3">
        {[0, 1, 2].map((i) => <Skeleton key={i} h="5.5rem" className="rounded-card" />)}
      </div>
    );
  else
    body = (
      <>
        {picks && (
          <SlotPicker d={d} when={when} setWhen={setWhen} sel={sel} setSel={setSel} at={at} setAt={setAt} problem={server.slot} />
        )}
        {bare && !picks && <p className="text-muted">{t("workflows.run_now.manual")}</p>}
        {picks && params.length > 0 && (
          <p className="flex gap-2 rounded-card border border-line-soft bg-surface-2 px-3 py-2 text-[12px] text-muted">
            <Icon name="info" className="mt-0.5 h-3.5 w-3.5" />
            {t("drawer.slot.note")}
          </p>
        )}
        {rerun?.slot && <p data-rerun-note className="rounded-card border border-line-soft bg-surface-2 px-3 py-2 text-[12px] text-muted">{t("drawer.rerun.note", { slot: utc(rerun.slot) })}</p>}
        {grouped(params).map((g) => (
          <div key={g.name} className="space-y-2.5">
            {g.name && <h3 data-param-group={g.name} className="pt-1 text-[13px] font-semibold text-ink">{g.name}</h3>}
            {g.params.map((p) => (
              <Field key={p.name} p={p} def={defOf(p)} value={value(p)} problem={problems.get(p.name) ?? ""} onChange={(v) => edit(p, v)} draft={(x) => (drafts.current[p.name] = x)} />
            ))}
          </div>
        ))}
        {dropped.map((name) => (
          <p key={name} className="font-mono text-[0.8rem] text-faint">
            {name} <span className="font-sans text-[0.72rem]">{t("drawer.dropped")}</span>
          </p>
        ))}
        {params.length > 0 && (
          <details open className="rounded-card border border-line-soft">
            <summary className="cursor-pointer px-3 py-2 text-[0.78rem] text-muted">{t("drawer.payload")}</summary>
            <div className="space-y-2 px-3 pb-3">
              <pre className="overflow-x-auto rounded-card border border-line-soft bg-canvas px-3 py-2 font-mono text-[0.75rem] text-ink-2"><code>{JSON.stringify(payload, null, 2)}</code></pre>
              <p className="text-[0.75rem] text-muted">{t("drawer.cli")}</p>
              <pre className="overflow-x-auto rounded-card border border-line-soft bg-canvas px-3 py-2 font-mono text-[0.75rem] text-ink-2">{cli}</pre>
            </div>
          </details>
        )}
        {formError && <p data-run-error role="alert" className="rounded-card border border-state-failed/40 bg-state-failed/10 px-3 py-2 text-[12.5px] text-state-failed">{formError}</p>}
      </>
    );

  const runIcon = <Icon name="play" className="h-3.5 w-3.5" />;
  const blocked = !d || !values || invalid > 0 || slotMissing || post.isPending;
  return (
    <dialog
      ref={ref}
      className={bare ? "modal" : "drawer"}
      aria-labelledby="run-title"
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onClick={(e) => e.target === ref.current && onClose()}
    >
      <form
        noValidate
        className="flex min-h-0 flex-1 flex-col"
        onKeyDown={onKey}
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {head}
        <div className={bare ? "dialog-body space-y-3 text-[13px]" : "flex-1 space-y-4 overflow-y-auto px-5 py-4"}>{body}</div>
        {bare ? (
          <footer className="dialog-foot">
            <button type="submit" disabled={blocked} className={buttonClass("primary", "regular")}>
              {runIcon}
              {t("workflows.run_now.run")}
            </button>
          </footer>
        ) : (
          <div className="flex items-center gap-3 border-t border-line px-5 py-3">
            <p aria-live="polite" className={`min-w-0 flex-1 truncate text-[12.5px] ${invalid ? "text-state-failed" : changed.length ? "text-warn" : "text-muted"}`}>
              {d && values ? status : ""}
            </p>
            <button type="button" onClick={reset} className="text-[12.5px] text-ink hover:underline">{t("drawer.reset")}</button>
            <button type="submit" disabled={blocked} className={buttonClass("primary", "regular")}>
              {runIcon}
              {t(rerun ? "drawer.button.rerun" : "drawer.button.run")}
              <kbd className="ml-1 rounded border border-current/30 px-1 font-sans text-[10px] opacity-70">⌘↵</kbd>
            </button>
          </div>
        )}
      </form>
    </dialog>
  );
}

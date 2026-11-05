import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { getJSON } from "../api/client";
import { useFormat, useT } from "../i18n";
import { Empty } from "../ui/card";
import { PageFrame } from "../ui/page-frame";
import { LoadError } from "../ui/query-boundary";
import { recalled, remember } from "../ui/remembered";

type Project = { slug: string; name: string; workflows: number; runs: number; createdAt: string };

// Code in the lead sentence: the catalog message names where they go, so a language can place them anywhere.
const EXAMPLE = "daily_ingest";
const FLAG = "brevis publish --project";
const grid = "grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4";
const card = "group rounded-card border border-line bg-surface p-5 transition-colors hover:border-line-strong";

function ProjectCard({ p }: { p: Project }) {
  const { t } = useT();
  const { date } = useFormat();
  return (
    <Link to={`/workflows?project=${encodeURIComponent(p.slug)}`} className={card}>
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="display truncate text-base text-ink">{p.name}</h3>
        <span className="shrink-0 font-mono text-[0.7rem] text-muted">{p.slug}</span>
      </div>
      <dl className="mt-4 flex gap-3 text-sm">
        <div>
          <dt className="text-[0.7rem] tracking-wide text-muted uppercase">{t("nav.workflows")}</dt>
          <dd className="display text-xl text-ink tabular-nums">{p.workflows}</dd>
        </div>
        <div>
          <dt className="text-[0.7rem] tracking-wide text-muted uppercase">{t("nav.runs")}</dt>
          <dd className="display text-xl text-ink tabular-nums">{p.runs}</dd>
        </div>
      </dl>
      <p className="mt-4 flex items-center justify-between text-xs text-muted">
        <span>{t("projects.since", { date: date(p.createdAt) })}</span>
        <span className="text-muted opacity-0 transition-opacity group-hover:opacity-100">{t("projects.open")} →</span>
      </p>
    </Link>
  );
}

// While the projects load the grid draws the real card with placeholder values (a ghost),
// as many as last time, so the loaded grid is the same height by construction.
const GHOST: Project = { slug: "project_slug", name: "project name", workflows: 0, runs: 0, createdAt: "2026-01-01T00:00:00Z" };

function Ghosts() {
  const [n] = useState(() => recalled("projects.cards", 3, 500));
  return (
    <>
      {Array.from({ length: n }, (_, i) => (
        <div key={i} className="sk-ghost contents" aria-hidden="true">
          <ProjectCard p={GHOST} />
        </div>
      ))}
    </>
  );
}

export default function ProjectsRoute() {
  const { t } = useT();
  const q = useQuery({ queryKey: ["projects"], queryFn: () => getJSON<Project[]>("/api/projects") });
  const data = q.data;
  useEffect(() => {
    if (data) remember("projects.cards", data.length);
  }, [data]);
  return (
    <PageFrame title={t("nav.projects")}>
      <p className="mb-4 max-w-3xl text-sm text-muted">
        {t("projects.lead", { example: EXAMPLE, flag: FLAG })
          .split(new RegExp(`(${EXAMPLE}|${FLAG})`))
          .map((part, i) => (i % 2 === 1 ? <code key={i} className="font-mono text-xs">{part}</code> : part))}
      </p>
      {q.isError ? (
        <LoadError onRetry={() => void q.refetch()} />
      ) : data && data.length === 0 ? (
        <Empty text={t("projects.empty")} hint="brevis publish --project my-project" />
      ) : (
        <div className={grid} aria-busy={!data} aria-hidden={!data || undefined}>
          {data ? data.map((p) => <ProjectCard key={p.slug} p={p} />) : <Ghosts />}
        </div>
      )}
    </PageFrame>
  );
}

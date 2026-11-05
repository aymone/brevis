import { useEffect, useState } from "react";
import { useT } from "../i18n";

// A connector's or a language's mark on a quiet tile, or a monogram where there
// is no mark: web/components/logo.go's registry, drawn by the server stylesheet's
// .cg classes. Paint says how the mark meets the theme.
type Paint = "cg-one" | "cg-multi" | "cg-dark" | "cg-light" | "cg-brand text-accent-strong" | "cg-multi cg-on-white";
const MARKS: Record<string, [title: string, paint: Paint]> = {
  airflow: ["Apache Airflow", "cg-one"], bigquery: ["Google BigQuery", "cg-one"], brevissql: ["brevis-sql", "cg-brand text-accent-strong"],
  clickhouse: ["ClickHouse", "cg-light"], dbt: ["dbt", "cg-one"], docker: ["Docker", "cg-one"], duckdb: ["DuckDB", "cg-light"],
  gcs: ["Google Cloud Storage", "cg-light"], geocoding: ["Google Maps", "cg-one"], go: ["Go", "cg-multi"], kafka: ["Apache Kafka", "cg-dark"],
  kubernetes: ["Kubernetes", "cg-one"], mysql: ["MySQL", "cg-one"], node: ["Node.js", "cg-multi"], pandas: ["pandas", "cg-multi cg-on-white"],
  polars: ["Polars", "cg-one"], postgres: ["PostgreSQL", "cg-multi"], pubsub: ["Google Pub/Sub", "cg-light"], python: ["Python", "cg-multi"],
  redis: ["Redis", "cg-multi"], redshift: ["Amazon Redshift", "cg-one"], s3: ["Amazon S3", "cg-one"], shell: ["GNU Bash", "cg-one"],
  snowflake: ["Snowflake", "cg-one"], spark: ["Apache Spark", "cg-one"], sqs: ["Amazon SQS", "cg-one"], terraform: ["Terraform", "cg-one"],
  vision: ["Google Cloud", "cg-one"],
};
const MONOGRAMS: Record<string, [title: string, mono: string]> = {
  file: ["Local files", "FS"], http: ["HTTP", "{}"], gateway: ["Brevis gateway", "→"], kinesis: ["Amazon Kinesis", "KS"], sftp: ["SFTP", "SF"],
};

let loaded: Record<string, string> | null = null;
let loading: Promise<void> | null = null;
function useMarks() {
  const [marks, setMarks] = useState(loaded);
  useEffect(() => {
    if (loaded) return;
    loading ??= import("./logo-marks").then((m) => void (loaded = m.marks));
    let live = true;
    void loading.then(() => live && setMarks(loaded));
    return () => void (live = false);
  }, []);
  return marks;
}

/** A key's name for a reader and a screen reader: a brand is its own name in every language; "file" and "gateway" are words, translated. */
export function useLogoTitle() {
  const { t } = useT();
  return (k: string) => (k === "file" || k === "gateway" ? t(`logo.${k}`) : (MARKS[k]?.[0] ?? MONOGRAMS[k]?.[0] ?? k));
}

/** Whether the registry draws this key (a step card shows a mark only for these). */
export const hasMark = (k: string) => k in MARKS || k in MONOGRAMS;

export function Logo({ k, size = 18, title: given }: { k: string; size?: 18 | 28 | 40; title?: string }) {
  const name = useLogoTitle()(k);
  const marks = useMarks();
  const mark = MARKS[k];
  const mono = MONOGRAMS[k];
  const letters = [...k].filter((c) => /\p{L}/u.test(c)).slice(0, 2).join("").toUpperCase();
  const title = given ?? name;
  if (mark) {
    return (
      <span className={`cg cg-${size} ${mark[1]}`} role="img" aria-label={title} title={title} dangerouslySetInnerHTML={{ __html: marks?.[k] ?? "" }} />
    );
  }
  return (
    <span className={`cg cg-mono cg-${size}`} role="img" aria-label={title} title={title}>
      {mono?.[1] ?? letters}
    </span>
  );
}

/** The marks of what a workflow runs in, side by side. */
export function Logos({ keys, size = 18 }: { keys: string[]; size?: 18 | 28 | 40 }) {
  return (
    <>
      {keys.map((k) => (
        <Logo key={k} k={k} size={size} />
      ))}
    </>
  );
}

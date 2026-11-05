// The names a person reads for a step's runtime or tool: the payload carries ids
// (stable keys shared with the YAML), and an id with no label reads as itself.
const LANG: Record<string, string> = {
  python: "Python", go: "Go", node: "Node.js", java: "Java", rust: "Rust", php: "PHP", ruby: "Ruby", dotnet: ".NET",
  sql: "SQL", shell: "Shell", dbt: "dbt", spark: "Spark", airbyte: "Airbyte", soda: "Soda", sqlmesh: "SQLMesh",
  meltano: "Meltano", dlt: "dlt", duckdb: "DuckDB", pandas: "pandas", polars: "Polars", airflow: "Airflow", terraform: "Terraform",
};
export const langLabel = (id: string) => LANG[id] ?? id;

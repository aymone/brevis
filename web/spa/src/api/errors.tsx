import { ApiError } from "./client";
import { useT } from "../i18n";

// The sentence for an API failure, in the reader's language. The server answers a code (internal/api/apierror.go) and
// the catalog says it; a code this app does not know, or none at all, reads as a plain failure. Add a code in both places.
const KEY: Record<string, string> = {
  // The session endpoints.
  invalid_credentials: "login.invalid",
  too_many_attempts: "login.locked",
  // Any endpoint.
  unavailable: "loaderror.text",
  not_found: "error.not_found",
  not_enabled: "error.not_enabled",
  invalid_request: "error.invalid_request",
  forbidden: "error.forbidden",
  // One action.
  run_just_started: "error.run_just_started",
  cancel_failed: "error.cancel_failed",
  schedule_failed: "error.schedule_failed",
  no_schedule: "error.no_schedule",
  not_written_by_brevis: "error.not_written",
  unknown_destination: "error.unknown_destination",
  sql_not_configured: "error.sql_not_configured",
  statement_too_long: "error.statement_too_long",
  refused: "sql.err.refused",
  no_connection: "sql.err.no_connection",
  run_failed: "sql.err.failed",
  // Try it on an endpoint.
  endpoint_not_configured: "endpoint.try.err.not_configured",
  endpoint_unbuilt: "endpoint.try.err.unbuilt",
  endpoint_unreachable: "endpoint.try.err.unreachable",
  endpoint_unreadable: "endpoint.try.err.unreadable",
  endpoint_too_large: "endpoint.try.err.too_large",
  endpoint_not_answer: "endpoint.try.err.not_answer",
};

/** The catalog key that says an error code; "error.generic" for one that is not known. */
export const errorKey = (code: string): string => KEY[code] ?? "error.generic";

/** What a reader is told about a failure: (e) => sentence. Anything that is not an ApiError is a plain failure. */
export function useErrorText() {
  const { t } = useT();
  return (e: unknown): string => {
    const code = e instanceof ApiError ? e.code : "";
    const vars = e instanceof ApiError && typeof e.body?.connection === "string" ? { connection: e.body.connection } : undefined;
    return t(errorKey(code), vars);
  };
}

/** An API failure said in the reader's language, inline: <ErrorText error={query.error} />. */
export function ErrorText({ error }: { error: unknown }) {
  const text = useErrorText();
  return <>{text(error)}</>;
}

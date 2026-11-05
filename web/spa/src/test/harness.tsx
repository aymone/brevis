import type { ReactElement } from "react";
import { render } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";

/** The cache main.tsx fills in boot(): screens never suspend on the session or the catalog. */
export function seededClient() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(["session"], { operator: "tester", lang: "en" });
  qc.setQueryData(["config"], { brand: { title: "T", subtitle: "", phrase: "", logo: "", attribution: "", css: "" }, version: "0", sql: false });
  // An empty catalog: every word shows as [key], which is also what the tests assert on.
  qc.setQueryData(["messages", "en"], { lang: "en", messages: {} });
  return qc;
}

export function renderApp(ui: ReactElement, { at = "/", qc = seededClient() } = {}) {
  return { qc, ...render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[at]}>{ui}</MemoryRouter></QueryClientProvider>) };
}

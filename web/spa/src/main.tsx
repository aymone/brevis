import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Shell } from "./shell";
import { boot } from "./boot";
import OverviewRoute from "./routes/overview";
import RunsRoute, { RunsIndex } from "./routes/runs";
import RunRoute from "./routes/run";
import WorkflowsRoute from "./routes/workflows";
import WorkflowRoute from "./routes/workflow";
import ProjectsRoute from "./routes/projects";
import StreamsRoute from "./routes/streams";
import StreamRoute from "./routes/stream";
import LoadsRoute from "./routes/loads";
import CatalogRoute from "./routes/catalog";
import EndpointsRoute from "./routes/endpoints";
import EndpointRoute from "./routes/endpoint";
import SqlRoute from "./routes/sql";
import LoginRoute from "./routes/login";
import { useT } from "./i18n";
import "./styles.css";

// EVERY SCREEN IS IN THE FIRST DOWNLOAD, on purpose. A screen's title, its
// description, its filters and its column headers are static, and static things
// never wait: a route that loaded its code on demand could only fill the gap with
// a skeleton of the whole page, and that is exactly what the screens must not do.
// What stays out of the download is what is heavy AND only part of a screen: the
// run's graph (React Flow) and the brand marks, which load inside their screens.

// Data stays fresh for 5s: moving between screens does not refetch what was
// just read, and a screen that comes back shows the old data while it renews.
const client = new QueryClient({ defaultOptions: { queries: { staleTime: 5_000, retry: 1 } } });

// The catch-all route: a path no screen answers.
function NotFound() {
  const { t } = useT();
  return <p>{t("app.not_found")}</p>;
}

await boot(client);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <Routes>
          {/* Outside the Shell: its own minimal layout, and the one screen an anonymous visitor may open. */}
          <Route path="login" element={<LoginRoute />} />
          <Route element={<Shell />}>
            <Route index element={<OverviewRoute />} />
            <Route path="runs" element={<RunsRoute />}>
              <Route index element={<RunsIndex />} />
              <Route path=":id" element={<RunRoute />} />
            </Route>
            <Route path="projects" element={<ProjectsRoute />} />
            <Route path="sql" element={<SqlRoute />} />
            <Route path="workflows" element={<WorkflowsRoute />} />
            <Route path="workflows/:slug" element={<WorkflowRoute />} />
            <Route path="ingest/streams" element={<StreamsRoute />} />
            <Route path="ingest/streams/:gateway/*" element={<StreamRoute />} />
            {/* The load is a drawer over the list: the list reads the child's params, so it stays mounted. */}
            <Route path="ingest/loads" element={<LoadsRoute />}>
              <Route path=":workflow/:step" />
            </Route>
            <Route path="data" element={<CatalogRoute />} />
            <Route path="data/target" element={<CatalogRoute />} />
            <Route path="data/endpoints" element={<EndpointsRoute />} />
            <Route path="data/endpoints/:api/:name" element={<EndpointRoute />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);

# The console app (web/spa)

React 19 + React Router + TanStack Query, built by Vite and served by the Go binary at `/`.

```bash
cd web/spa
npm ci            # exactly the lockfile
```

## Develop

```bash
make up                 # from the repo root: the API on :8080 (the dev server proxies to it)
npm run dev             # http://localhost:5173/
BREVIS_API=http://localhost:8098 npm run dev   # against another API
```

## Check

| command | what it runs | needs |
|---|---|---|
| `npm run typecheck` | `tsc --noEmit` over `src/`, `e2e/` and the configs | nothing |
| `npm run lint` | oxlint: `react/rules-of-hooks` (error), `react/exhaustive-deps` (warning) | nothing |
| `npm run test` | Vitest, jsdom: the app's logic and a few render tests, about 2 s | nothing |
| `npm run e2e` | Playwright (Chromium) against a running engine, about 50 s | an engine, see below |

CI runs the first three on every PR (`spa` job) and the e2e suite when the engine or the console changes (`e2e` job).
Both live in `.github/workflows/test.yml`.

oxlint and not ESLint: typescript-eslint does not yet accept TypeScript 7, and these two rules are all the config
enforces. `rules-of-hooks` is the one that finds a hook placed after an early return (the bug that blanked the
Workflows page on a search with no result).

### Unit tests

`src/**/*.test.ts(x)`, next to the code they cover. Stream, hooks, i18n, API client, remembered shapes, filters, the
test-event checker and `boot` are tested as logic; `routes/workflows.test.tsx` renders the real screen against a mocked
`fetch`. `src/test/` holds the shared setup. Layout is NOT tested here (jsdom has none): that is the e2e suite's job.

### End-to-end tests

The specs drive the embedded dist, the real API, the real event stream and a real scheduler, so they need an engine.
Postgres on any port, then from the repo root:

```bash
export BREVIS_ENV=local BREVIS_HTTP_ADDR=:8098 BREVIS_METRICS_ADDR= \
       BREVIS_DATABASE_URL='postgres://USER:PASS@HOST:PORT/brevis_e2e?sslmode=disable'   # a scratch database
go build -o bin/brevis ./cmd/brevis
bin/brevis migrate up
bin/brevis publish examples/hello.yaml examples/list-params.yaml web/spa/e2e/fixtures/e2e_nightly.yaml web/spa/e2e/fixtures/e2e_single.yaml
bin/brevis serve &
bin/brevis scheduler --interval 2s --concurrency 4 &

cd web/spa
npx playwright install chromium       # once
BASE_URL=http://localhost:8098 npm run e2e
```

`BASE_URL` defaults to `http://localhost:8080` (what `make up` serves). The specs create what they need through the
public API (`POST /api/workflows/{slug}/trigger`) and skip with a stated reason when a precondition is absent (no
workflow, no schedule, no SQL service, no streams). `E2E_REQUIRE=1` (set in CI) turns a missing engine or workflow into
a failure instead of a skip. Failures keep a trace and a screenshot: `npx playwright show-trace test-results/.../trace.zip`,
`npx playwright show-report`.

Thresholds are fixed in the specs: CLS < 0.02, no URL read twice, 0 REST requests on an idle screen, no console error,
no sideways overflow. The idle check watches 5 s, not the 25 s of the old scratch script, to stay inside the time budget.

## The dist is committed

`dist/` is checked in and embedded by `spa.go`. The engine's image has no Node, so the build cannot happen where the
binary is made; the repo carries the result instead. To keep that honest, `.github/scripts/generated-check.sh` runs
`npm ci && npm run build` and fails the `Engine` job if the tree differs: change anything under `src/`, run
`npm run build`, commit `dist/` with it. Test-only files and devDependencies used by tests do not reach the bundle, so
adding a test never changes `dist/`.

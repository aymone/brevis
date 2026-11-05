# Brevis: notes for AI coding agents and humans

Brevis is a Go engine (API, scheduler, runner), a gateway, a SQL API, a public Go
SDK, a Python library and a React console. `CONTRIBUTING.md` has the project's
rules and the reasons; this file is what an agent gets wrong without being told.

## Language

- **All code is American English (en-US)**: comments, identifiers, schema, JSON
  keys, file names, test names, fixtures, commit messages, docs. `color`,
  `canceled`, `behavior`, `license`.
- The only Portuguese in the repo is (a) the console's translation catalogs,
  `web/i18n/locales/*.json`, and (b) the legacy READERS listed in
  `CONTRIBUTING.md` ("The project speaks English"): code that still accepts an old
  spelling so data written by an older release keeps working. Do not add to that
  list; a rename is a new English name, plus a reader for the old one only if a
  released version used it. Fixtures stay Portuguese only when the Portuguese is
  the point: a proper noun (São Paulo), an IANA zone (`America/Sao_Paulo`), an
  accent-handling test.
- **Before you call anything done, re-read what you touched**: strings, comments,
  identifiers, test names, fixtures, docs. Then run
  `./.github/scripts/repo-language-check.sh` and `./.github/scripts/ui-language-check.sh`.
  Both are nets, not proofs.
- Catalog text (`web/i18n/locales/*.json`, 11 languages) is reviewed the way a
  native-level localizer would: one term per concept across the catalog, and the
  `_review` note in a translated file stays honest about how far it was reviewed.
  `web/i18n/keys_test.go` and `web/i18n/i18n_test.go` enforce that keys exist, are
  used, and that every language has them all.

## Architecture rules agents get wrong

- **The server has no language.** It never renders a sentence for a person: it
  sends a message key with its data (`web/i18n/msg.go`: `Msg`, `M`, `MN`) or an
  error code (`internal/api/apierror.go`), and the console says it
  (`web/spa/src/i18n.ts`, `web/spa/src/api/errors.tsx`). A new code or key is added
  on both sides, plus every catalog.
- **Migrations are append-only.** Never edit a migration that has run (comments
  aside); a schema rename takes a new migration. `migrations/00002`..`00032` keep
  the old Portuguese names because they ran on that schema (migration `00033`
  renames it).
- **Two or more Go modules, not one**: the root (the engine), `gateway/`, `sql/`,
  `sdk/` (and its submodules), `examples/`, `cmd/brevis-sdk/`. The engine never
  imports the SDK (`.github/scripts/engine-weight.sh`); the SDK's drivers are
  values in `from/` and `to/` so a consumer only compiles what it imports
  (`.github/scripts/pruning-check.sh`).
- `web/assets/app.css` and `web/spa/dist/` are generated and committed: change
  `web/assets/app.src.css` or `web/spa/src/`, then `make generate`.

## Commands

```bash
go build ./... && go vet ./...        # per module: root, gateway/, sql/, sdk/, examples/
go test ./...                          # the engine, at the root; the DB tests skip without Postgres
make test-int                          # DB tests too: `make up` first, they run with -p 1 because
                                       # several packages TRUNCATE the same database
BREVIS_TEST_DATABASE_URL=postgres://... go test -p 1 ./internal/...   # the same, on a database of your own
(cd sdk && go test ./...)              # likewise gateway/ and sql/
(cd web/spa && npm ci && npm run typecheck && npm run lint && npm run test)
(cd web/spa && E2E_REQUIRE=1 CI=1 npm run e2e)     # Playwright; see the "Console e2e" job
./.github/scripts/python-check.sh      # the Python library
make generate                          # CSS and the React app
```

Run the package-scoped tests that answer the question (`go test ./web/i18n/`),
not the whole suite, until the end. CI (`.github/workflows/test.yml`) runs:
`Engine` (gofmt, build, vet, `go test -race ./...`, `generated-check.sh`,
`engine-weight.sh`), `Integration` (the database and driver tests, and
`pruning-check.sh`, `cli-docs-check.sh`), `Console app` (typecheck, lint,
vitest), `Console e2e`, `Python library`, and `Repo-wide gates`
(`repo-language-check.sh`, `ui-language-check.sh`, `helm-check.sh`, and the
others in `.github/scripts/`, each of which says at its top why it exists).

## Branches, commits, versions

- Commits and PR titles are `type: description` (`feat`, `fix`, `refactor`,
  `docs`, `test`, `chore`, ...). `.github/PULL_REQUEST_TEMPLATE.md` is the PR
  checklist; a change carries a test that fails without it.
- Each deliverable has its own `VERSION` file (`VERSION`, `gateway/VERSION`,
  `sql/VERSION`) and the release workflows refuse a tag that does not match it.
  The changelogs (`CHANGELOG.md` for the SDK, `CHANGELOG-motor.md` for the engine,
  and the per-module ones) record decisions as written: new entries in English,
  older ones are not rewritten.

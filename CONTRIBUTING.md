# Contributing to Brevis

## The project speaks English

Code, comments, identifiers, the database schema, JSON keys, file names, error
messages, commit messages, tests, plans and documentation are written in
**American English** (en-US: `color`, `canceled`, `behavior`, `license`). That
is not a style preference: a contributor in Berlin or Osaka reads the same
comment as the person who wrote it, and a comment nobody can read is a comment
that stops being maintained.

The server has no language of its own. It answers an error **code**
(`internal/api/apierror.go`) or a **message key** with its data (`web/i18n/msg.go`:
`Msg`, `M`, `MN`), and the console says the sentence in the reader's language.
All of the console's words live in the catalogs, `web/i18n/locales/*.json`
(eleven languages, English first). `web/i18n/keys_test.go` enforces that every key
the code asks for exists and that every key is used, and `web/i18n/i18n_test.go`
that the catalogs are complete and consistent.
Catalog text is written and reviewed the way a native-level localizer would:
one term per concept, and the `_review` note in each translated file says
honestly how far that review went.

**The only Portuguese left in this repository is (a) the console's translation
catalogs (`pt-BR.json`, with `es.json` and the other nine languages beside it)
and (b) the short list of legacy readers below.** A legacy reader is code that
still accepts an old Portuguese spelling, so data or consumers written by an
older release keep working. Each one says in its own comment why the old
spelling exists and when it can go, and each is allowlisted, with its reason, in
`.github/scripts/repo-language-check.sh`. A new Portuguese name is never added:
a rename is a new English name plus, if a released version used the old one, a
reader for the old one.

| Legacy spelling | Where it is read | What reads it | Goes when |
|---|---|---|---|
| Stage keys `indice`, `nome`, `estado`, `em`, `numeros` in `task_runs.stages` | `Stage.UnmarshalJSON` in `internal/infrastructure/postgres/taskruns.go` and `internal/application/execution/stages.go`, and the `numeros` fallback of `ReadRepo.volume` in `internal/infrastructure/postgres/insights.go` | the engine reading rows written before the keys went English (and by a pod of the old engine during a rolling update) | every engine writes the English keys and the task-run retention has passed |
| The SDK's marker lines `{"tipo":"etapa","nome","estado","versao","em"}` | `stageCollector.line` in `internal/application/execution/stages.go` | the engine reading the stdout of a fetcher built with SDK v0.47.0 or earlier | no fetcher below v0.48.0 is in production |
| Param keys `Nome`, `Tipo`, `Descricao` | `Param.UnmarshalJSON` in `internal/domain/workflow/param.go` | the engine reading `runs.definition` snapshots of runs that can still be retried | the oldest retryable run was created by an engine that writes the English keys |
| Checkpoint depot: `_completo`, `parte-NNNNN.ndjson` and the manifest keys `versao`, `registros`, `partes`, `numeros`, `gravado_em` | `sdk/internal/checkpoint/checkpoint.go` | an SDK resuming a depot an earlier SDK wrote | no such depot can still be resumed (the depots' retention) |
| Environment variables `BREVIS_AUTH_USUARIO`, `BREVIS_AUTH_SENHA_HASH`, `BREVIS_AUTH_SEGREDO`, `BREVIS_POD_MANTER_EM_FALHA`, `BREVIS_SDK_LIMITE_INLINE` | `renamed` in `internal/config/config.go`, `EnvIntRenamed` in `sdk/internal/core/config.go` | an installation or fetcher that still sets the old name (it warns and names the new one) | the next major version, after the warning has shipped for a release or two |
| The session cookie name `brevis_sessao` (now `brevis_session`) | `LegacyCookieName` and `sessionCookie` in `internal/auth/auth.go` | the gate honoring a session signed in under the old name before an upgrade, and sign-out clearing both | `SessionLifetime` after the first release that writes `brevis_session` (every old cookie has expired) |
| The drawer hashes `#publicar` and `#novo` (now `#publish` and `#new`) | `LEGACY_DRAWERS` in `web/spa/src/ui/drawer.tsx` | a bookmark or a shared link to the endpoints' publish drawer or the streams' new-stream drawer, written before 0.43 | no such link is in use (they were never documented; drop it with the next major version) |
| The command alias `brevis marca` | `cmdBrand` in `cmd/brevis/main.go` | a script that calls the command by its released name | v1 |
| Deprecated SDK names `from.CampoJSON`, `postgres.ParaJSON`, `postgres.ParaJSONComOID`, `mysql.ParaJSON`, `mysql.ComParseTime`, `load.ComoCriar`, `load.CriarPorSQL`, `load.CriarPorSchema`, `load.PlanoDeCriacao` | each `Deprecated:` comment in `sdk/` | consumers that compiled against a published SDK | v1 |
| Migrations `00002`..`00032` and `internal/infrastructure/postgres/testdata/before00033/` | `migrations/`, `upgrade_00033_test.go` | a database that ran them on the old Portuguese schema; migration `00033` renames it | the oldest supported database is past `00033`, and the upgrade test goes with them |
| The object-name prefix `parte-` | `sdk/to/files.go`, `sdk/persist/persist.go` | nothing reads it: it is what a WRITER still names its parts, because landing zones already hold such objects and consumers glob them | consumers have moved to a glob that accepts `part-` too |

Migrations are append-only: a file that has run is never edited (comments aside),
and renaming anything in the schema takes a new migration.

The gates, which run in CI (the `Repo-wide gates` job of `test.yml`) and are one
command each:

```bash
./.github/scripts/repo-language-check.sh   # comments, strings, Go and Python identifiers, SQL, file names
./.github/scripts/ui-language-check.sh     # the English catalog, which the console shows by default
```

`repo-language-check.sh` is a net, not a proof: it judges every comment, every Go
string literal, the identifiers and file names against a Portuguese vocabulary and
a list of British spellings, and Portuguese made only of words its lists lack
gets through. Add the word to the list the day one does, and say why. Reading
what you wrote -- the strings, comments, names and test names you touched -- is
still part of the work.

The two changelogs predate this rule in part and are kept as they were written:
they record decisions made on a date, and rewriting a record is not the same as
translating a project. New entries are in English.

Everything else in the repository describes the **current state**. What was done
is in the git history, and what is planned is in the issues — a document that
tracks either one goes stale in a week and then misleads. `docs/` holds no
dated specs for that reason.

## What a change has to carry

**A test that would fail without it.** For a bug fix, the test is the proof the
bug existed — not "added tests", but which behavior it pins down.

**Proof that the test bites.** Revert your fix and check the test goes red. A
test that cannot fail is worse than no test: it buys confidence it has not
earned. Several tests in this repository were found to be unable to fail, and
each was found this way.

**A test that configures nothing, when the feature has a default.** Two
features shipped switched off in one week, and both times every test in the
package set the field under test — so the default was the one path nothing
exercised. A consumer migrating 34 tasks that each asked for three attempts
three minutes apart got three attempts inside three seconds, with every test
green.

**A comment saying why, where the why is not obvious.** The diff shows what
changed. The comment exists for the person who, two years from now, will look at
a strange-looking line and want to "simplify" it. Most of the comments in this
repository name the incident that produced the line.

## Running it

```bash
go test ./...          # the engine, at the root
cd sdk && go test ./...  # the SDK, a separate module

make generate          # regenerates web/ (Tailwind, pinned, and the React app)
```

Two modules, two `go.mod`. The engine does not import the SDK, and
`.github/scripts/engine-weight.sh` enforces it: the API and the scheduler are one
7 MB binary, and the data drivers live in the task pods.

Before opening a pull request:

```bash
golangci-lint run ./...                     # in both modules
./.github/scripts/generated-check.sh        # web/ artifacts are current
./.github/scripts/engine-weight.sh          # the engine stays lean
./.github/scripts/pruning-check.sh          # a consumer only compiles what it imports
./.github/scripts/repo-language-check.sh    # comments, messages, names and file names are English
./.github/scripts/ui-language-check.sh      # the English catalog is English
./.github/scripts/helm-check.sh             # the chart renders correctly
```

## The warehouse is local, and CI says so

**BigQuery is not in CI and will not be**, decided 2026-10-08. Three reasons,
not one: this repository is public, so a long-lived GCP key in its secrets is
a standing target through every third-party action the workflows pull; the
project on hand holds a client's real data; and the emulator already covers
the protocol.

The emulator runs load jobs, so it covers the write path too: rows landing,
the merge not doubling, a JSON null stored as SQL NULL. What it does **not**
cover is how the real service behaves: quota, DDL concurrency, how JSON is
stored, column-case folding. So CI runs the emulated layer and then says, in a
step of its own, that it did not run the other:

> SKIPPED, which is green: nothing below this line was proven against a real
> warehouse.

**The other half of that decision is a command**, because otherwise "we run it
locally" is a sentence rather than something that happens:

```bash
make warehouse-up                 # postgres, mysql, an S3 API, floci, redis, memcached

export BREVIS_IT_PROJECT=my-dev-project
export BREVIS_IT_DATASET=brevis_it      # must exist, with a table expiry
export BREVIS_IT_BUCKET=my-bucket       # for the GCS load strategy
gcloud auth application-default login

make warehouse                    # every test, and A SKIP IS A FAILURE
```

**A skip is a failure there, which is the opposite of CI and the point.** CI
tolerates a skip because it has no credentials; that run exists to prove
everything ran, so a test that did not run is the one outcome it must refuse —
`go test` exits 0 on a skip and prints green, which is how nine integration
tests once rotted unnoticed.

Run it before any release that touches BigQuery. It creates and drops tables,
datasets and objects: point it at a project you are willing to lose.

## Things this project refuses

**A public field that does nothing.** If it is not read, it is not there.

**A number that is always zero.** It teaches people to skip the field, and then
nobody sees the one time it is real.

**Dropping data in silence.** Failing loudly is a one-line problem; a silent
divergence is a duplicate that turns up weeks later in a report.

**Guessing where the answer is not knowable.** Refusing, naming what is missing,
beats a default that is right half the time.

## The SDK's dependencies

Three, and adding a fourth needs an argument. Go prunes dependencies by package
imported, never by field used — which is why the drivers are values in `from/`
and `to/`: a fetcher that never imports the BigQuery destination never builds it.
`pruning-check.sh` is what keeps that true.

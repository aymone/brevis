#!/usr/bin/env bash
# The repository is in English -- comments and the messages a person reads.
#
# CONTRIBUTING.md has said so since the sweep of 2026-09-06, and until
# 2026-09-10 the only automated check was ui-language-check.sh, which covers
# the message catalog and only user-visible text. Everything else was policed by
# reading, which found 19 Portuguese strings in production code -- including
# ":rotating_light: Falha no pipeline", which went to a customer's Slack, and
# two error messages in the PUBLIC SDK that consumers read.
#
# This covers what that one does not:
#
#   1. comment lines, repository-wide
#   2. strings inside fmt.Errorf, errors.New, http.Error, panic and slog calls
#      -- the prose a person reads when something breaks
#   3. EVERY string literal in non-test Go and in the Python library, and the
#      messages of t.Error/t.Fatal in tests (the second pass, below)
#
# WHAT IS DELIBERATELY OUT, with the reason, because a check whose exclusions
# are unexplained is a check somebody deletes:
#
#   - CHANGELOG.md and CHANGELOG-motor.md. They record decisions made on a
#     date, and rewriting a record is not translating a project. New entries
#     are in English.
#   - The MESSAGES in *_test.go, but not their comments. 471 t.Errorf strings
#     are still Portuguese; nobody outside the project reads them, so they were
#     left for a sweep of their own rather than mixed into a release. Test
#     COMMENTS are checked like any other: excluding the whole file was too
#     broad, and it hid a Portuguese comment in process_test.go for a day.
#   - The console's message catalogs, web/i18n/locales/*.json. JSON is not an
#     extension this reads; the English one is checked by ui-language-check,
#     and pt-BR.json and es.json are Portuguese and Spanish on purpose.
#   - The legacy spellings code reads for data an older release wrote -- the
#     stage keys of postgres.Stage, the old checkpoint depot's names, the
#     `Nome`/`Tipo`/`Descricao` param keys. Identifiers, keys and file names
#     are English since migration 00033; these are single words read for
#     compatibility, each documented where it lives.
set -uo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

python3 - <<'PY'
import re, pathlib, sys

# Portuguese function words. Short ones that are also English (`no`, `da`, `a`)
# are absent on purpose: this fires on real leaks, not on arguments.
# Function words only. The nouns are deliberately absent: `etapas`, `saida`,
# `ambiente`, `origem`, `criado_em`, `duracao_ms` and `tentativa` are
# IDENTIFIERS in this repository -- column names, struct fields and payload
# keys that CONTRIBUTING.md keeps Portuguese on purpose -- so a comment
# explaining one of them is correct English prose about a Portuguese name. A
# detector that flagged those would be turned off within a week, which is the
# argument ui-language-check.sh already makes.
# `tipo` is absent for the same reason as the nouns above: it is a WIRE
# FORMAT key -- the SDK spoke `{"tipo":"etapa"}` up to v0.47.0 -- so the
# comments explaining it are correct English prose about a Portuguese name.
# `são` is absent: it fired on "São Paulo" in a timezone test whose comment is
# correct English. A gate that flags a city name is a gate somebody switches
# off, and this project's cron tests will keep naming that city.
WORDS = """que nao não para uma por como sem isso vem foi pelo mais
já está sendo ser tem dos das aos nas nos qual onde porque então aqui cada
mesmo assim muito quando entre sobre depois antes ainda apenas seu sua seus
suas fazer feito precisa deve pode nenhum nenhuma todos todas invalido
primeira primeiro ultima ultimo
desconhecido desconhecida parenteses
formulario instancia interrompido encerrado esperava ilegivel proveniencia
disponiveis indisponivel
declaracao desistencia templatavel satisfaz publica"""
PT = re.compile(r"\b(" + "|".join(WORDS.split()) + r")\b", re.I)

# Code spans are stripped first: `task_runs.etapas` and `image:` are names, and
# a name is not prose.
CODE = re.compile(r"`[^`]*`")

# Where prose a person reads lives.
# org.opencontainers.image.* is here because a Dockerfile LABEL is neither a
# comment nor a Go error call, so it fell through BOTH branches below -- and it
# is the most widely read prose this repository publishes: registries print it
# beside the image. A Portuguese description shipped on the worker image that
# way and was found by eye, not by this gate.
#
# Describe() is here for the same reason and was found the same way. It is a
# PUBLIC method on every driver in the SDK, its string reaches consumers through
# logs and through the run's Result, and `from.Many` answered
# `many: %d sources, a primeira %s` for as long as it existed.
FALA = re.compile(r"fmt\.Errorf|errors\.New|http\.Error|panic\(|slog\.\w+|"
                  r"\.Info\(|\.Warn\(|\.Error\(|\.Debug\(|"
                  r"org\.opencontainers\.image\.(?:description|title)")

# Describe() is checked by TRACKING ITS BODY, not by matching a line.
#
# It is a public method on every driver, its string reaches consumers through
# the logs and through the run's Result, and `from.Many` answered
# `many: %d sources, a primeira %s` for as long as it existed. A line-based rule
# cannot see it: the signature line holds no string, and the return is two lines
# down. gofmt puts the closing brace at column 0, which is what makes this
# reliable enough to gate on.
DESCRIBE = re.compile(r"^func \([^)]*\) Describe\(\) string \{")
STRING = re.compile(r'"((?:[^"\\]|\\.){6,})"')
COMENTARIO = re.compile(r"^\s*(//|#|--)\s*(.+)$")

FORA = ("/vendor/", "/node_modules/", "/.git/", "/testdata/",
        "CHANGELOG.md", "CHANGELOG-motor.md", "NOTES.md")
EXTS = {".go", ".md", ".yml", ".yaml", ".sh"}
# Makefile and Dockerfile carry operator-facing echoes and have no extension --
# three Portuguese ones lived in the Makefile until 2026-09-10 because every
# check keyed on a suffix.
SEM_EXTENSAO = {"Makefile", "Dockerfile"}

ruins = []
for p in sorted(pathlib.Path(".").rglob("*")):
    s = str(p)
    conhecido = p.suffix in EXTS or p.name in SEM_EXTENSAO
    if not p.is_file() or not conhecido or any(x in s for x in FORA):
        continue
    dentro, escopo = False, False
    for n, linha in enumerate(p.read_text(encoding="utf-8", errors="replace").split("\n"), 1):
        # `escopo` is THIS line; `dentro` is the lines after it. They differ for
        # the one-line body, which is both -- and `from.Files` is exactly that
        # shape. A first version tracked only `dentro`: it either missed the
        # one-liner entirely or, setting the flag on it, left the flag stuck for
        # the rest of the FILE.
        if DESCRIBE.match(linha):
            escopo = True
            dentro = not linha.rstrip().endswith("}")
        elif dentro and linha == "}":
            escopo, dentro = False, False
        else:
            escopo = dentro

        c = COMENTARIO.match(linha)
        if c:
            m = PT.search(CODE.sub(" ", c.group(2)))
            if m:
                ruins.append((s, n, m.group(0), "comment", c.group(2)[:60]))
            continue
        if s.endswith("_test.go") or not (escopo or FALA.search(linha)):
            continue
        for raw in STRING.findall(linha):
            m = PT.search(CODE.sub(" ", raw))
            if m:
                ruins.append((s, n, m.group(0), "message", raw[:60]))

# ---------------------------------------------------------------------------
# THE SECOND PASS: every string literal, not only the ones in a call kind.
#
# The pass above looks at strings on the same line as fmt.Errorf, slog and the
# like. It never saw `Message: "pod aguardando: " + reason` (a run-log line),
# `errors.New("slack respondeu 503")` in a test, `"slots truncados no ciclo"`
# split over two lines, or anything held in a variable. This pass tokenizes the
# Go source properly and judges EVERY string literal in non-test code, and the
# strings handed to t.Error/t.Fatal/t.Log/t.Skip in tests. Python's library gets
# the same for every string token.
#
# A literal is Portuguese when it
#   - carries a Portuguese/Spanish diacritic, or
#   - has two or more words and holds a word from PT2, a list of words that are
#     not English and are not identifiers in this repository, or
#   - holds a British spelling (CONTRIBUTING.md: en-US). `canceled`, `color`.
#
# It is a net, not a proof: Portuguese made only of words this list lacks gets
# through, and the list grows the day one does -- the way WORDS above did.
#
# WHAT IS ALLOWED, each with its reason, so nobody deletes the list unread:
ALLOW = [
    # The language switcher names each language in ITS OWN language, which is
    # what a reader who cannot read the current one needs to find it.
    ("web/i18n/negotiate.go", None),
    # SQL text: judged by the SQL_RE skip below, not by path. The schema is
    # English since migration 00033; the old migrations still name the old
    # columns, because they ran on a schema that spoke Portuguese.
    # The SDK's legacy stage keys ({"tipo":"etapa"}, v0.47.0 and earlier) are a
    # wire format read for compatibility; single words, so they never reach the
    # two-word rule, and this file also quotes the old payloads in prose.
    ("internal/application/execution/stages.go", None),
    # `marca` is a Portuguese alias of the `brand` command, kept so a script
    # that spoke it does not break; a flag/alias name, not prose.
    ("cmd/brevis/main.go", ["marca"]),
    # sql/example takes a day-first date on purpose: it demonstrates a param
    # transform that accepts a Brazilian date and rewrites it to ISO.
    ("sql/example/main.go", ["02/01/2006"]),
    # São Paulo is a city, and the cron tests name it on purpose.
    ("internal/scheduler/slot_test.go", ["São Paulo"]),
]
# Test fixtures that are Portuguese on purpose (the data the code is tested
# against: São Paulo, accented CSV, Portuguese column names) are not in a t.*
# call, so they are never judged; only t.* messages are, in test files.

W1 = set(WORDS.split())
PT2 = set("""nao para isso mais como pelo pela pelos pelas seu sua seus suas ainda apenas quando
depois antes sobre entre cada mesmo assim muito onde porque qual deve devem pode podem
fazer feito deveria devia esperado esperada esperava esperando recebeu aceitou ficou
devolveu carregou gravou perdeu falhou rodou deixou retornou estava estavam tinha
nunca sempre todos todas nenhum nenhuma alguma algum outros outras uma dos das nos nas
foi tem sem por aos ao ja esta estao entao aqui isto aquele aquela essa esse
mensagem arquivo caminho erro erros falha falhas sucesso aguardando respondeu
cancelado cancelada tentativa tentativas execucao registro registros linhas linha
tarefa passo etapa etapas lendo criando abrindo conectando publicando serializando
gravando reivindicando enfileirando iniciando estourou entrou terminou truncado
truncados restante proximos ciclo ciclos padrao invalido invalida invalidos ausente
ausentes vazio vazia nome chave valor nada tudo coisa diretorio coluna colunas tabela
resposta pagina paginas requisicao requisicoes conexao banco dados
nem mas pois porem contudo enquanto durante apos ate desde""".split())
# Short words that are also English (de, em, do, no, as, um, com ...) are NOT in
# the list on purpose. WEAK below are words that are also identifiers here
# (column names, struct fields): one of them alone does not condemn a literal,
# two together do. `sao` is absent: it is the city in "Sao Paulo".
WEAK = set("nome chave valor nada tudo coisa tabela banco dados erro erros linha linhas passo etapa etapas tarefa registro registros coluna colunas pagina paginas resposta sucesso falha falhas ja ate nem mas pois nos nas tem sem por ao aos".split())
BRIT = re.compile(r"\b(cancell(?:ed|ing)|colours?|behaviours?|licences?|initialis\w+|serialis\w+|"
                  r"deserialis\w+|analys(?:e|es|ed|ing)|centred|normalis\w+|organis\w+|recognis\w+|"
                  r"optimis\w+|favour\w*|catalogue\w*|honour\w*|summaris\w+|authoris\w+|"
                  r"customis\w+|minimis\w+|randomis\w+|unlabelled|labelled)\b", re.I)
DIAC = re.compile(r"[ãõçáéíóúâêôàüñ]", re.I)
SQL_RE = re.compile(r"\b(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|FROM|WHERE)\b")
TCALL = {"Error", "Errorf", "Fatal", "Fatalf", "Skip", "Skipf", "Log", "Logf"}

def go_tokens(src):
    """Yield (kind, text, line) for strings, identifiers and punctuation."""
    i, n, line = 0, len(src), 1
    while i < n:
        c = src[i]
        if c == "\n":
            line += 1; i += 1
        elif c in " \t\r":
            i += 1
        elif src.startswith("//", i):
            j = src.find("\n", i)
            i = n if j < 0 else j
        elif src.startswith("/*", i):
            j = src.find("*/", i + 2)
            j = n if j < 0 else j + 2
            line += src.count("\n", i, j); i = j
        elif c == '"':
            j = i + 1
            while j < n and src[j] != '"':
                j += 2 if src[j] == "\\" else 1
            yield ("str", src[i + 1:j], line)
            i = j + 1
        elif c == "`":
            j = src.find("`", i + 1)
            j = n if j < 0 else j
            yield ("str", src[i + 1:j], line)
            line += src.count("\n", i, j); i = j + 1
        elif c == "'":
            j = i + 1
            while j < n and src[j] != "'":
                j += 2 if src[j] == "\\" else 1
            i = j + 1
        elif c.isalpha() or c == "_":
            j = i
            while j < n and (src[j].isalnum() or src[j] == "_"):
                j += 1
            yield ("id", src[i:j], line); i = j
        else:
            yield ("p", c, line); i += 1

def go_literals(src, is_test):
    """The string literals worth judging: all of them in code, the t.* call
    arguments in a test."""
    stack, prev3, prev2, prev = [], None, None, None  # stack of "is this call a t.* call"
    for kind, text, line in go_tokens(src):
        if kind == "p" and text == "(":
            stack.append(prev is not None and prev[0] == "id" and prev[1] in TCALL
                         and prev2 is not None and prev2[1] == "."
                         and prev3 is not None and prev3[1] in ("t", "b", "tb", "tt"))
        elif kind == "p" and text == ")":
            if stack: stack.pop()
        elif kind == "str":
            if not is_test or any(stack):
                yield line, text
        prev3, prev2, prev = prev2, prev, (kind, text)

def judge(raw):
    s = raw
    plain = CODE.sub(" ", s)
    if SQL_RE.search(s):
        return None
    if BRIT.search(plain):
        return BRIT.search(plain).group(0)
    if DIAC.search(plain):
        return DIAC.search(plain).group(0)
    words = re.findall(r"(?<![\w/.%$#:=-])[a-zA-Z]{2,}(?![\w/.:=-])", plain.lower())
    if len(words) < 2:
        return None
    hits = [w for w in words if w in PT2 or w in W1]
    strong = [w for w in hits if w not in WEAK]
    if strong or len(hits) >= 2:
        return (strong or hits)[0]
    return None

def allowed(path, raw):
    for suffix, needles in ALLOW:
        if path.endswith(suffix) and (needles is None or any(x in raw for x in needles)):
            return True
    return False

def py_literals(src):
    import io, tokenize
    try:
        for t in tokenize.generate_tokens(io.StringIO(src).readline):
            if t.type == tokenize.STRING:
                yield t.start[0], t.string
    except (tokenize.TokenError, IndentationError):
        return

for p in sorted(pathlib.Path(".").rglob("*")):
    s = str(p)
    if not p.is_file() or any(x in s for x in FORA) or "/web/spa/" in "/" + s:
        continue
    if p.suffix == ".go":
        lits = go_literals(p.read_text(encoding="utf-8", errors="replace"), s.endswith("_test.go"))
    elif p.suffix == ".py" and s.startswith("lib/") and "/tests/" not in s:
        lits = py_literals(p.read_text(encoding="utf-8", errors="replace"))
    else:
        continue
    for n, raw in lits:
        m = judge(raw)
        if m and not allowed(s, raw):
            ruins.append((s, n, m, "string literal", raw[:60].replace("\n", " ")))

if ruins:
    for s, n, palavra, tipo, texto in ruins:
        print(f"::error file={s},line={n}::Portuguese in a {tipo}: {palavra!r} in {texto!r}")
    print(f"\n{len(ruins)} line(s). The repository is English -- see CONTRIBUTING.md.")
    print("If a word here is a false positive, remove it from WORDS in this")
    print("script and say why in the commit.")
    sys.exit(1)
print("✅ comments and messages are in English")
PY

#!/usr/bin/env bash
# The repository is in American English -- comments, names and the messages a
# person reads.
#
# CONTRIBUTING.md has said so since the sweep of 2026-09-06, and until
# 2026-09-10 the only automated check was ui-language-check.sh, which covers
# the message catalog and only user-visible text. Everything else was policed by
# reading, which found 19 Portuguese strings in production code -- including
# ":rotating_light: Falha no pipeline", which went to a customer's Slack, and
# two error messages in the PUBLIC SDK that consumers read. Then the Go
# identifiers, the schema and the file names were renamed to English, and
# nothing kept them there: a `TestSemToken`, a `criado_em` in a new migration or
# a `dados.csv` would have been green.
#
# What it judges, in order:
#
#   1. every comment, in every language the repository holds (Go, TypeScript,
#      Python, SQL, shell, YAML, CSS, Markdown, Dockerfile, Makefile), for
#      Portuguese function words and for British spellings
#   2. strings inside fmt.Errorf, errors.New, http.Error, panic and slog calls,
#      and the body of every Describe() -- the prose a person reads when
#      something breaks
#   3. EVERY string literal in non-test Go and in the Python library, and the
#      messages of t.Error/t.Fatal in tests
#   4. NAMES: Go and Python identifiers (test and non-test: functions, types,
#      methods, fields, variables, constants), SQL identifiers (migrations
#      newer than 00033 and every other .sql file, and the SQL held in Go
#      strings), and file names -- each judged by the WORDS of the name
#      (camelCase and snake_case split) against PT_ID, and against the British
#      spellings. Migrations 00002..00032 may keep the old identifiers (they ran
#      on that schema), but their comments and COMMENT ON text must be English.
#
# WHAT IS DELIBERATELY OUT, with the reason, because a check whose exclusions
# are unexplained is a check somebody deletes:
#
#   - CHANGELOG.md and CHANGELOG-motor.md. They record decisions made on a
#     date, and rewriting a record is not translating a project. Entries from
#     0.40.0 on are English; the older ones stay as written.
#   - The console's message catalogs, web/i18n/locales/*.json. JSON is not an
#     extension this reads; the English one is checked by ui-language-check, and
#     the other ten are translations on purpose.
#   - Anything under a `testdata/` directory: fixtures are data. The frozen
#     schema (testdata/before00033) is the one that matters.
#   - The legacy spellings code READS for data an older release wrote -- the
#     stage keys of postgres.Stage, the old checkpoint depot's names, the
#     `Nome`/`Tipo`/`Descricao` param keys, the deprecated SDK aliases, the
#     deprecated environment variables, `brevis marca`. Each is listed in
#     ID_ALLOW (or ALLOW) below with what reads what, and in CONTRIBUTING.md
#     with when it can go.
#
# It is a net, not a proof: Portuguese made only of words its lists lack gets
# through, and the list grows the day one does. Reading what you wrote is still
# part of the work (CLAUDE.md).
set -uo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

python3 - <<'PY'
import re, pathlib, sys, subprocess, tokenize, io

# ---------------------------------------------------------------------------
# THE VOCABULARY.
#
# Three lists, because three different questions are asked of three different
# kinds of text, and one list cannot answer them all.
#
#   WORDS    Portuguese FUNCTION words and verbs, for PROSE (comments and docs).
#            Nouns are mostly absent on purpose: a comment about a legacy
#            spelling ("rows written before the keys went English carry
#            indice, nome, estado") is correct English prose about a Portuguese
#            name, and a gate that flagged it would be switched off within a
#            week. The legacy nouns are named in LEGACY below.
#   PT_ID    Portuguese words for IDENTIFIERS, file names and SQL names. Here
#            nouns are exactly the point: `etapa`, `tarefa`, `saida`, `criado`
#            are what the rename sweep removed, and one coming back is the leak.
#   BRIT     British spellings. CONTRIBUTING.md: en-US.
#
# Short words that are also English (`no`, `da`, `a`, `sem` as in semaphore,
# `data`, `base`, `final`, `pico`, `visto`, `troca`) are absent from every list
# on purpose: this fires on real leaks, not on arguments.
# ---------------------------------------------------------------------------
WORDS = """que nao não para uma por como sem isso vem foi pelo mais
já está sendo ser tem dos das aos nas nos qual onde porque então aqui cada
mesmo assim muito quando entre sobre depois antes ainda apenas seu sua seus
suas fazer feito precisa deve pode nenhum nenhuma todos todas invalido
primeira primeiro ultima ultimo
desconhecido desconhecida parenteses
formulario instancia interrompido encerrado esperava ilegivel proveniencia
disponiveis indisponivel
declaracao desistencia templatavel satisfaz publica
continua chega sobe vira ficava rodam podem parecer coisa inteira inteiro quem
dentro cima lugar errado verdade secao derrubar escolhe monta montar apaga
avisar atualiza persiste registra entra falhar devolve devolvem recebe usa
sobrescrevem herda ausentes ausente vazio vazia vazios
silencio janela carregando culpando deixaria deixou escreve conferida
aparece tambem porem contudo enquanto durante apos desde ate_
tentar rodar rodando funcionando igual passa passam
somar mostra mostram procura procuram entrega entregam existem
acontece recusa recusou
pelos pelas nossa nosso dele dela deles delas
voce voces eles elas isto aquele aquela essa esse esta_ este_"""
WORDS = WORDS.replace("ate_", "").replace("esta_", "").replace("este_", "")
PT = re.compile(r"\b(" + "|".join(WORDS.split()) + r")\b", re.I)

# Code spans are stripped first: `task_runs.stages` and `image:` are names, and
# a name is not prose.
CODE = re.compile(r"`[^`]*`")

# THE LEGACY READERS. The only Portuguese a comment may QUOTE, because each is
# a spelling the code still READS for data or consumers an older release
# produced, and its comment says why. CONTRIBUTING.md lists them with what reads
# what and when each can go; the code that reads them is in ID_ALLOW below.
LEGACY = set("""indice nome estado em numeros tipo etapa etapas versao registros
partes parte gravado criado completo descricao usuario senha segredo manter
falha marca campo""".split())

# Where prose a person reads lives, inside a Go file: error and log calls.
# org.opencontainers.image.* is here because a Dockerfile LABEL is neither a
# comment nor a Go error call, and it is the most widely read prose this
# repository publishes: registries print it beside the image.
#
# Describe() is here for the same reason and was found the same way. It is a
# PUBLIC method on every driver in the SDK, its string reaches consumers through
# logs and through the run's Result, and `from.Many` answered
# `many: %d sources, a primeira %s` for as long as it existed.
FALA = re.compile(r"fmt\.Errorf|errors\.New|http\.Error|panic\(|slog\.\w+|"
                  r"\.Info\(|\.Warn\(|\.Error\(|\.Debug\(|"
                  r"org\.opencontainers\.image\.(?:description|title)")

# Describe() is checked by TRACKING ITS BODY, not by matching a line: the
# signature line holds no string, and the return is two lines down. gofmt puts
# the closing brace at column 0, which is what makes this reliable enough to
# gate on.
DESCRIBE = re.compile(r"^func \([^)]*\) Describe\(\) string \{")
STRING = re.compile(r'"((?:[^"\\]|\\.){6,})"')

# What is never read: vendored and generated trees, the changelogs (they record
# decisions made on a date; new entries are English, older ones stay as
# written), the console's message catalogs (Portuguese and Spanish on purpose,
# and the other nine languages besides: ui-language-check.sh and the keys test
# judge them), the compiled console bundle, and the frozen schema the upgrade
# test restores.
FORA = ("/vendor/", "/node_modules/", "/.git/", "/testdata/",
        "CHANGELOG.md", "CHANGELOG-motor.md", "NOTES.md",
        "web/i18n/locales/", "web/spa/dist/", "web/assets/app.css",
        "package-lock.json", "go.sum")
COMMENT_EXTS = {".go", ".md", ".yml", ".yaml", ".sh", ".ts", ".tsx", ".js", ".mjs",
                ".css", ".py", ".sql", ".toml", ".tpl", ".gitignore"}
# Makefile and Dockerfile carry operator-facing echoes and have no extension --
# three Portuguese ones lived in the Makefile until 2026-09-10 because every
# check keyed on a suffix.
SEM_EXTENSAO = {"Makefile", "Dockerfile", ".gitignore", ".dockerignore", ".air.toml"}

# British spellings (CONTRIBUTING.md: en-US). One regex for prose and one for
# the PARTS of an identifier, which is where `Colours()` and
# `TestTheEnvelopeRefusesWhatItCannotHonour` hid.
BRIT_STEMS = ("recogn|serial|deserial|initial|optim|summar|normal|organ|author|custom|minim|"
              "random|final|util|sanit|synchron|visual|token|general|special|real|prioriti|"
              "parametr|material|capital|categor|standard|stabil|local|memor|emphas")
BRIT_ODD = (r"cancell(?:ed|ing|able)|colour\w*|behaviour\w*|licence[sd]?|catalogue\w*|"
            r"artefacts?|centre[sd]?|grey\w*|neighbour\w*|favour\w*|honour\w*|"
            r"judgement\w*|modelling|travelled|whilst|amongst|labelled|unlabelled|"
            r"analys(?:e|ed|ing)|programme")
_BRIT = BRIT_ODD + r"|(?:" + BRIT_STEMS + r")is(?:e|es|ed|ing|ation|ations|er|ers|able)"
BRIT = re.compile(r"\b(?:" + _BRIT + r")\b", re.I)
BRIT_PART = re.compile(_BRIT, re.I)

# ---------------------------------------------------------------------------
# COMMENTS: every comment in every language this repository holds, not only the
# ones that start a line. `x := 1 // a primeira` and `/* ... */` blocks were
# invisible to a gate that read `^\s*(//|#|--)`.
# ---------------------------------------------------------------------------
def comments_of(path, text):
    ext = path.rsplit(".", 1)[-1] if "." in path.rsplit("/", 1)[-1] else ""
    name = path.rsplit("/", 1)[-1]
    out = []
    if ext in ("md", "txt"):
        # Prose is all comment, and the fenced samples are written for a reader
        # too.
        return [(i, l) for i, l in enumerate(text.split("\n"), 1)]
    c_like = ext in ("go", "ts", "tsx", "js", "mjs", "css")
    hash_like = ext in ("py", "sh", "yml", "yaml", "toml", "tpl", "gitignore") or name in SEM_EXTENSAO
    sql = ext == "sql"
    i, n, line = 0, len(text), 1
    if c_like or sql:
        while i < n:
            ch = text[i]
            if ch == "\n":
                line += 1; i += 1
            elif c_like and text.startswith("//", i):
                j = text.find("\n", i); j = n if j < 0 else j
                out.append((line, text[i + 2:j])); i = j
            elif sql and text.startswith("--", i):
                j = text.find("\n", i); j = n if j < 0 else j
                out.append((line, text[i + 2:j])); i = j
            elif text.startswith("/*", i) and (c_like or sql):
                j = text.find("*/", i + 2); j = n if j < 0 else j + 2
                for k, l in enumerate(text[i + 2:j].split("\n")):
                    out.append((line + k, l))
                line += text.count("\n", i, j); i = j
            elif ch == '"' or (ch == "'" and sql):
                q = ch; j = i + 1
                while j < n and text[j] != q and text[j] != "\n":
                    j += 2 if text[j] == "\\" else 1
                i = j + 1
            elif ch == "`" and c_like:
                j = text.find("`", i + 1); j = n if j < 0 else j
                line += text.count("\n", i, j); i = j + 1
            else:
                i += 1
    if hash_like:
        for k, l in enumerate(text.split("\n"), 1):
            m = re.search(r"(^|\s)#(?!!)\s?(.*)$", l)
            if m:
                out.append((k, m.group(2)))
    return out

ruins = []

def tracked():
    files = subprocess.run(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
                           capture_output=True, text=True, check=True).stdout.split("\0")
    return [f for f in files if f and pathlib.Path(f).is_file()]

FILES = tracked()

for s in FILES:
    p = pathlib.Path(s)
    conhecido = p.suffix in COMMENT_EXTS or p.name in SEM_EXTENSAO
    if not conhecido or any(x in "/" + s for x in FORA):
        continue
    src = p.read_text(encoding="utf-8", errors="replace")
    for n, c in comments_of(s, src):
        plain = CODE.sub(" ", c)
        m = PT.search(plain)
        if m and m.group(0).lower() not in LEGACY:
            ruins.append((s, n, m.group(0), "Portuguese in a comment", c.strip()[:60]))
        b = BRIT.search(plain)
        if b:
            ruins.append((s, n, b.group(0), "British spelling in a comment", c.strip()[:60]))
    dentro, escopo = False, False
    for n, linha in enumerate(src.split("\n"), 1):
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
        if s.endswith("_test.go") or not (escopo or FALA.search(linha)):
            continue
        for raw in STRING.findall(linha):
            m = PT.search(CODE.sub(" ", raw))
            if m:
                ruins.append((s, n, m.group(0), "Portuguese in a message", raw[:60]))

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
# The legacy readers' single-word keys ("tipo", "etapa", `marca`, "parte-") never
# reach the two-word rule, so they need no entry; the places that read them are
# listed in CONTRIBUTING.md.
ALLOW = [
    # The language switcher names each language in ITS OWN language, which is
    # what a reader who cannot read the current one needs to find it.
    ("web/i18n/negotiate.go", None),
    # São Paulo is a city, and the cron tests name it on purpose.
    ("internal/scheduler/slot_test.go", ["São Paulo"]),
]
# Test fixtures that are Portuguese on purpose (the data the code is tested
# against: São Paulo, accented CSV) are not in a t.* call, so they are never
# judged; only t.* messages are, in test files.

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
# the list on purpose. WEAK below are words that are also names in places (a
# column called `nome` in a legacy reader): one of them alone does not condemn a
# literal, two together do. `sao` is absent: it is the city in "Sao Paulo".
WEAK = set("nome chave valor nada tudo coisa tabela banco dados erro erros linha linhas passo etapa etapas tarefa registro registros coluna colunas pagina paginas resposta sucesso falha falhas ja ate nem mas pois nos nas tem sem por ao aos".split())
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

ALLOW_USED = set()

def allowed(path, raw):
    for i, (suffix, needles) in enumerate(ALLOW):
        if path.endswith(suffix) and (needles is None or any(x in raw for x in needles)):
            ALLOW_USED.add(i)
            return True
    return False

def py_literals(src):
    try:
        for t in tokenize.generate_tokens(io.StringIO(src).readline):
            if t.type == tokenize.STRING:
                yield t.start[0], t.string
    except (tokenize.TokenError, IndentationError):
        return

for s in FILES:
    if any(x in "/" + s for x in FORA) or "/web/spa/" in "/" + s:
        continue
    p = pathlib.Path(s)
    if p.suffix == ".go":
        lits = go_literals(p.read_text(encoding="utf-8", errors="replace"), s.endswith("_test.go"))
    elif p.suffix == ".py" and s.startswith("lib/") and "/tests/" not in s:
        lits = py_literals(p.read_text(encoding="utf-8", errors="replace"))
    else:
        continue
    for n, raw in lits:
        m = judge(raw)
        if m and not allowed(s, raw):
            ruins.append((s, n, m, "Portuguese in a string literal", raw[:60].replace("\n", " ")))

# ---------------------------------------------------------------------------
# THE THIRD PASS: NAMES. Go identifiers (test and non-test), the Python
# library's, SQL identifiers, and file names.
#
# The first two passes read prose. The sweep that made the schema, the Go
# identifiers and the file names English left no way to keep them so: a
# `TestSemToken`, a `criado_em` in a new migration or a `dados.csv` would have
# been green. A name is judged by its WORDS -- `getCriadoEm` is `get`, `criado`,
# `em` -- against PT_ID, so `ErrorCreated` is fine and `ErrorCriado` is not,
# without a dictionary and without flagging `data` or `base`.
# ---------------------------------------------------------------------------
# ENGLISH HOMOGRAPHS. A word that is Portuguese AND English (or an acronym, or a
# term of art) is not evidence of Portuguese: `data` is data, `final` is final,
# `sem` is a semaphore. Each is listed with the reason, because a silent
# exclusion is how a gate rots. They are removed from PT_ID below.
HOMOGRAPHS = {
    "pico": "pico- prefix (picoseconds)",
    "visto": "English word (a visa)",
    "troca": "appears in vendor API names",
    "data": "English: data",
    "base": "English: base",
    "final": "English: final",
    "sensor": "English: sensor",
    "sem": "a semaphore",
    "ini": "INI files",
    "dia": "diameter/diagram-style prefix",
    "mes": "an acronym in vendor APIs",
    "ano": "anode and similar compounds",
    "fim": "an acronym",
    "pai": "an acronym",
    "sal": "salt, a term of art for password hashing",
    "alfa": "alpha in vendor names",
    "tela": "English: a fabric",
    "sul": "ambiguous",
    "dos": "DOS",
    "ate": "English: ate",
    "era": "English: era",
    "para": "English prefix",
    "uma": "a name",
    "com": "a TLD and COM ports",
    "mesa": "English: mesa",
    "tempo": "English: tempo",
    "hora": "English: hora",
    "campo": "a surname in vendor data",
    "casos": "ambiguous",
    "mesmo": "ambiguous",
    "fila": "ambiguous",
    "dado": "ambiguous",
    "usa": "USA",
    "ja": "ja, the Japanese language code",
    "novo": "ambiguous",
    "eco": "eco- prefix",
    "grande": "Rio Grande and similar",
    "agora": "English: agora",
    "alvos": "ambiguous",
    "curto": "ambiguous",
}
PT_ID = set("""
etapa etapas estagio estagios fase fases tarefa tarefas passo passos
erro erros falha falhas sucesso aviso avisos alerta alertas
cancelar cancelado cancelada cancelamento publicar publicado publicada ler leitura
leituras leitor escrever escrita buscar busca listar lista criar criado criada
atualizar atualizado atualiza atualizada remover removido apagar apaga apagado
apagador excluir excluido executar execucao execucoes executado executada
tentativa tentativas tentar filas saida saidas entrada entradas
nome nomes tipo tipos estado estados ordem valor valores chave chaves
carga cargas agenda agendas agendamento mensagem mensagens resposta respostas
requisicao requisicoes conexao conexoes banco dados tabelas coluna colunas
linha linhas arquivo arquivos caminho caminhos diretorio pasta senha usuario
segredo operador painel filtro filtrar contagem contar contagens resumo
relatorio pendente rodando executando falhou concluido ativo inativo pausado
atraso prazo horario hoje ontem amanha dias semana minuto minutos
segundo segundos duracao inicio iniciado iniciar terminado terminar parar
retomar pausar esperar espera aguardar verdade falso vazio vazia cheio novo nova
velho antigo proximo proxima anterior ultimo ultima primeiro primeira todos nenhum
algum qualquer sempre nunca ainda depois antes durante entre sobre
tem processo quero transicionar disparar coletor existe limite ja eventos
projeto concorrente bruto bruta exigido podar conteudo dominio quer efeito prox
notificador autenticado voo evento codigo historico levas raiz avisar recortar
exceto querido truncado truncar recusa segunda retrato escritos modo viu
folga mesclar esperada achado colide metodo sufixo bloco blocos intervalo
causa diz nao comprimido comprime esquema formato prefixo pela congelado
aplica espaco longo meu objeto extremo recebeu sobra expandir desligar
restaurar carrega pacotes procura gravador gravar grava gravado inseguro
compara completa libera ocupar ocupado monta montar tudo maior reservado
colado aparar exemplo assinatura assinar emissor venenos veneno aceita
legitimo escalar uso visitar sobrepor transporte seguindo seguidores falsa preso
morto permite outra interrompe preparar procurado varrer predicado candidatas
classificar tecnologia seguintes segurar quantos fica degradar vez vence quebrar
impressos decodificador renovar guardado guardador caso escreve limpa resultado
descobertas contadora declarado exato canonicos semente usada ruim direto digestos
semantico extraiu preservar literais grupos sonda vendas pequeno reduzir
contaminado leva guarda corpo conflitos pausa unico palavra renomeado
indice numeros versao registros partes parte
extrair estavel coleta cobre cobrir lote lotes chegou sobe desce vira faz roda
fonte destino origem
pronto clima nada falador buscando pagina paginas compilando ruido misto paralelo
demais muitos inventado reticulando velho achada achado nova semente
""".split()) - set(HOMOGRAPHS)

# ID_ALLOW: (path suffix, the words that file may contain, WHY). One entry per
# legacy reader or deprecated name; a file not here has no Portuguese in a name.
ID_ALLOW = [
    ("internal/domain/workflow/param.go", {"nome", "tipo", "descricao"},
     "Param.UnmarshalJSON reads the Portuguese keys of param specs that runs.definition snapshots still hold"),
    ("sdk/load/table.go", {"criar", "criacao"},
     "deprecated aliases ComoCriar, CriarPorSQL, CriarPorSchema and PlanoDeCriacao, kept because they shipped in a published release"),
    ("sdk/load/alias_test.go", {"criar", "criacao"},
     "the test that guards those deprecated aliases"),
]
# SQL that talks to the OLD schema on purpose, with the reason. Everything else
# that runs against the database speaks the English schema.
SQL_ALLOW = [
    ("internal/infrastructure/postgres/legacy_schema_test.go", "restores the schema as it was before migration 00033"),
    ("internal/infrastructure/postgres/upgrade_00033_test.go", "the upgrade test of migration 00033: old schema in, English schema out"),
    ("internal/infrastructure/postgres/failed_ends_test.go", "migrates down to a revision before 00033, where the columns are still Portuguese"),
    ("internal/infrastructure/postgres/index_test.go", "migrates down to a revision before 00033, where the columns are still Portuguese"),
]
FILE_ALLOW = [
    ("internal/infrastructure/postgres/testdata/before00033/",
     "the frozen schema as it was before migration 00033: the upgrade test runs migrations 00002..00032 against it"),
]

def parts_of(ident):
    out = []
    for seg in ident.split("_"):
        out += re.findall(r"[A-Z]+(?![a-z])|[A-Z]?[a-z]+", seg)
    return [x.lower() for x in out]

def pt_parts(ident):
    return [w for w in parts_of(ident) if w in PT_ID]

def brit_parts(ident):
    return [w for w in parts_of(ident) if BRIT_PART.fullmatch(w)]

# Words that are Portuguese in a TEST NAME or a short test phrase (`sem token`,
# `TestSemToken`) but are English names elsewhere (`sem` is a semaphore, `com` a
# port), so they stay out of PT_ID and are only judged where a name is a
# sentence.
SENTENCE_PT = set("sem com uma dos das nas nos pelo pela".split())
PHRASE = re.compile(r"[a-z0-9' ,._-]{3,60}")
# a string that is one name or phrase, in a test: a fixture word, a case key, a subtest name

# Test files whose fixtures ARE the legacy spellings, so the single words in them
# are data. Each guards a legacy reader listed in CONTRIBUTING.md.
FIXTURE_ALLOW = [
    ("sdk/checkpoint_test.go", "a depot written before the rename: parte-00000.ndjson and its Portuguese manifest keys"),
    ("sdk/internal/checkpoint/format_test.go", "the manifest reader's legacy format"),
    ("internal/application/execution/stages_test.go", "the stage reader's legacy marker lines"),
    ("internal/application/execution/stages_end_to_end_test.go", "the legacy marker line, read end to end"),
    ("internal/application/workflow/unknown_test.go", "`tipo` is the deliberately unknown key of a spec"),
    ("sdk/to/files_object_test.go", "the `parte-` object prefix landing zones already hold"),
    ("internal/config/config_test.go", "the deprecated Portuguese environment variables"),
    ("sdk/internal/core/metadata_test.go", "the deprecated SDK environment variable"),
    ("internal/infrastructure/postgres/legacy_schema_test.go", "the pre-00033 schema"),
    ("internal/infrastructure/postgres/upgrade_00033_test.go", "the upgrade test of migration 00033"),
    ("internal/infrastructure/postgres/failed_ends_test.go", "a revision before 00033"),
    ("internal/infrastructure/postgres/index_test.go", "a revision before 00033"),
]

USED = set()

def id_allowed(path, words):
    for i, (suffix, allowed_words, _) in enumerate(ID_ALLOW):
        if path.endswith(suffix) and all(w in allowed_words for w in words):
            USED.add(i)
            return True
    return False

def judge_ident(path, ident, line, what):
    ws = pt_parts(ident)
    if re.match(r"(Test|Benchmark|Example|Fuzz)[A-Z]", ident):
        ws += [w for w in parts_of(ident) if w in SENTENCE_PT]
    if ws and not id_allowed(path, ws):
        ruins.append((path, line, ws[0], f"Portuguese in a {what}", ident))
    bs = brit_parts(ident)
    if bs:
        ruins.append((path, line, bs[0], f"British spelling in a {what}", ident))

GO_KEYWORDS = set("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var".split())

def py_names(src):
    try:
        for t in tokenize.generate_tokens(io.StringIO(src).readline):
            if t.type == tokenize.NAME:
                yield t.start[0], t.string
    except (tokenize.TokenError, IndentationError):
        return

SQL_STRIP = re.compile(r"--[^\n]*|/\*.*?\*/|'(?:[^']|'')*'|\$\$.*?\$\$|\"[^\"]*\"", re.S)
SQL_WORD = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")
COMMENT_ON = re.compile(r"COMMENT\s+ON\s+[^;]*?\s+IS\s+'((?:[^']|'')*)'", re.I | re.S)

def migration_number(s):
    m = re.match(r"migrations/(\d+)_", s)
    return int(m.group(1)) if m else None

def sql_identifiers(text):
    # the identifiers of a SQL text: what is left after comments and strings
    # are removed (a quoted identifier is kept: a Portuguese column is one)
    t = re.sub(r"--[^\n]*|/\*.*?\*/|'(?:[^']|'')*'", " ", text, flags=re.S)
    return [(text[:0], w) for w in SQL_WORD.findall(t)]

def judge_sql(path, text, offset_line=0, whole=True):
    seen = set()
    cleaned = re.sub(r"--[^\n]*|/\*.*?\*/|'(?:[^']|'')*'", lambda m: re.sub(r"[^\n]", " ", m.group(0)), text, flags=re.S)
    for ln, l in enumerate(cleaned.split("\n"), 1):
        for w in SQL_WORD.findall(l):
            ws = pt_parts(w)
            if ws and (w, ln) not in seen:
                seen.add((w, ln))
                ruins.append((path, offset_line + ln, ws[0], "Portuguese in a SQL identifier", w))
            bs = brit_parts(w)
            if bs and (w, ln) not in seen:
                seen.add((w, ln))
                ruins.append((path, offset_line + ln, bs[0], "British spelling in a SQL identifier", w))

def judge_file_name(s):
    if any(s.startswith(pref) for pref, _ in FILE_ALLOW):
        return
    if s.startswith("migrations/") and (migration_number(s) or 99) <= 32:
        return  # renamed to English in the commit of migration 00033
    for comp in s.split("/"):
        base = comp.rsplit(".", 1)[0] if "." in comp[1:] else comp
        base = re.sub(r"^\d+_", "", base)
        words = [w for chunk in re.split(r"[^A-Za-z0-9]+", base) for w in parts_of(chunk)]
        ws = [w for w in words if w in PT_ID]
        bs = [w for w in words if BRIT_PART.fullmatch(w)]
        if ws:
            ruins.append((s, 1, ws[0], "Portuguese in a file name", comp))
        if bs:
            ruins.append((s, 1, bs[0], "British spelling in a file name", comp))

for s in FILES:
    p = pathlib.Path(s)
    if "/node_modules/" in "/" + s or s.startswith(("web/spa/dist/", "web/i18n/locales/")) or s in ("go.sum",) \
            or p.suffix in (".svg", ".woff2", ".png", ".ico", ".jpg", ".want", ".sum", ".json", ".csv", ".txt"):
        if p.suffix not in (".json", ".csv", ".txt") or s.startswith(("web/spa/dist/", "web/i18n/locales/")) \
                or "/node_modules/" in "/" + s:
            continue
    judge_file_name(s)
    if p.suffix == ".go":
        src = p.read_text(encoding="utf-8", errors="replace")
        is_test = s.endswith("_test.go")
        for kind, text, line in go_tokens(src):
            if kind == "id" and text not in GO_KEYWORDS:
                judge_ident(s, text, line, "identifier")
            elif kind == "str" and is_test and "\n" not in text and PHRASE.fullmatch(text):
                # a subtest name, a table-case key or a fixture word: a short
                # lowercase string. SENTENCE_PT only counts in a phrase, where
                # `sem` is a word and not a semaphore.
                hit = [w for w in re.findall(r"[a-z]+", text) if w in PT_ID or (" " in text.strip() and w in SENTENCE_PT)]
                if hit and not allowed(s, text) and not any(s.endswith(f) for f, _ in FIXTURE_ALLOW):
                    ruins.append((s, line, hit[0], "Portuguese in a test name or case key", text))
            elif kind == "str" and SQL_RE.search(text) and not s.startswith("sql/") \
                    and not any(s.endswith(f) for f, _ in SQL_ALLOW):
                # SQL the engine runs: its names must be the English schema.
                judge_sql(s, text, line - 1)
    elif p.suffix == ".py":
        src = p.read_text(encoding="utf-8", errors="replace")
        for line, name in py_names(src):
            judge_ident(s, name, line, "identifier")
    elif p.suffix == ".sql" and not s.startswith("internal/infrastructure/postgres/testdata/before00033/"):
        src = p.read_text(encoding="utf-8", errors="replace")
        num = migration_number(s)
        if num is None or num > 33:
            judge_sql(s, src)  # 00033 is the rename: it names every old identifier once, to replace it
        # COMMENT ON text is prose, whatever the migration: judged like a comment
        for m in COMMENT_ON.finditer(src):
            txt = m.group(1)
            hit = PT.search(CODE.sub(" ", txt)) or BRIT.search(txt)
            if hit and hit.group(0).lower() not in LEGACY:
                ruins.append((s, src[:m.start()].count("\n") + 1, hit.group(0), "Portuguese in a COMMENT ON text", txt[:60]))

# A DOC COMMENT THAT NAMES A STALE PORTUGUESE IDENTIFIER. The rename sweep
# changed `func (r *Repo) Buscar` to `Get` and left "// Buscar le um Run." above
# it, in a language no word list of function words would catch.
DECL = re.compile(r"^(?:func (?:\([^)]*\) )?|type |var |const )([A-Za-z_]\w*)")
for s in FILES:
    if not s.endswith(".go") or any(x in "/" + s for x in FORA):
        continue
    block = []
    for n, l in enumerate(pathlib.Path(s).read_text(encoding="utf-8", errors="replace").split("\n"), 1):
        if l.startswith("//"):
            block.append(l)
            continue
        m = DECL.match(l)
        if m and block:
            first = block[0][2:].strip().split(" ")[0].rstrip(":,.")
            if first and first != m.group(1) and re.fullmatch(r"[A-Za-z_]\w*", first) \
                    and [w for w in parts_of(first) if w in PT_ID] and not id_allowed(s, [w for w in parts_of(first) if w in PT_ID]):
                ruins.append((s, n, first, "Portuguese in a doc comment naming a stale identifier", m.group(1)))
        block = []

for i, (suffix, _) in enumerate(ALLOW):
    if i not in ALLOW_USED:
        ruins.append((suffix, 1, suffix, "stale ALLOW entry (nothing in it needs the exception any more)", ""))

for i, (suffix, _, why) in enumerate(ID_ALLOW):
    if i not in USED:
        ruins.append((suffix, 1, suffix, "stale ID_ALLOW entry (nothing in it needs the exception any more)", why[:50]))

if ruins:
    ruins = sorted(set(ruins))
    for s, n, palavra, tipo, texto in ruins:
        print(f"::error file={s},line={n}::{tipo}: {palavra!r} in {texto!r}")
    print(f"\n{len(ruins)} line(s). The repository is English -- see CONTRIBUTING.md.")
    print("If a word here is a false positive, add it to HOMOGRAPHS (or remove it")
    print("from WORDS/PT_ID) in this script and say why in the commit; a legacy")
    print("reader goes in ID_ALLOW with what reads what.")
    sys.exit(1)
print("✅ comments, messages, identifiers, SQL and file names are in English")
PY

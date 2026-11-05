package postgres_test

import (
	"regexp"
	"strings"
)

// legacyNames maps the identifiers the schema had before 00033 to the ones it
// has now. Migrations 00012 and 00013 carry a backfill that these tests run
// against the CURRENT schema, so the old names in that SQL have to be read as
// the new ones. The migrations themselves stay as they were written: they ran
// on a schema that spoke Portuguese, and 00033 renames what they made.
var legacyNames = map[string]string{
	"alertas": "alerts", "tipo": "kind", "canal": "channel", "tentativas": "attempts",
	"erro": "error", "criado_em": "created_at", "iniciado_em": "started_at",
	"terminado_em": "finished_at", "definicao": "definition", "etapas": "stages",
	"sdk_versao": "sdk_version", "saida": "output", "em": "run_created_at",
	"linhas": "rows", "registros": "records", "ignorados": "ignored",
	"bytes_saida": "bytes_out", "bytes_entrada": "bytes_in", "paginas": "pages",
}

var legacyWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z_0-9]*`)

// asCurrentSchema rewrites the legacy identifiers of a migration's SQL to the
// current ones, leaving quoted text alone (the JSON keys are data).
func asCurrentSchema(sql string) string {
	var out strings.Builder
	inQuote := false
	for i := 0; i < len(sql); {
		c := sql[i]
		if !inQuote && strings.HasPrefix(sql[i:], "--") {
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				end = len(sql) - i
			}
			out.WriteString(sql[i : i+end])
			i += end
			continue
		}
		if c == '\'' {
			inQuote = !inQuote
		} else if !inQuote {
			if w := legacyWord.FindString(sql[i:]); w != "" {
				if n, ok := legacyNames[w]; ok {
					out.WriteString(n)
				} else {
					out.WriteString(w)
				}
				i += len(w)
				continue
			}
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}

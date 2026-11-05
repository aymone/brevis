package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	wf "brevis.sh/brevis/internal/domain/workflow"
	"brevis.sh/brevis/web/i18n"
)

func (u *UI) registerTriggerAPI(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/workflows/{slug}/trigger", u.triggerJSON)
}

// maxTriggerBody bounds the JSON a trigger reads: a few parameters, not a payload.
const maxTriggerBody = 1 << 20

// triggerBody is POST /api/workflows/{slug}/trigger: the Run drawer's form as
// JSON. A param is a string, number, boolean or a list of them; Slot is an
// RFC 3339 instant or the browser's datetime-local; FromRun names the run a
// rerun repeats.
type triggerBody struct {
	Params  map[string]any `json:"params"`
	Slot    string         `json:"slot"`
	FromRun string         `json:"from_run"`
}

// paramString is a JSON param value as the string every param travels as: a
// list joins its non-empty items with commas, exactly what paramsFromForm does
// with the old form's boxes.
func paramString(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	case json.Number:
		return x.String(), nil
	case []any:
		items := make([]string, 0, len(x))
		for _, e := range x {
			if _, nested := e.([]any); nested {
				return "", fmt.Errorf("a list holds values, not lists")
			}
			s, err := paramString(e)
			if err != nil {
				return "", err
			}
			if s != "" {
				items = append(items, s)
			}
		}
		return strings.Join(items, ","), nil
	}
	return "", fmt.Errorf("a param is a string, number, boolean or list")
}

// checkParams is the trigger form's validation, field by field: what
// Workflow.Resolver refuses as one sentence, said where it belongs. The rules
// are the definition's own (Param.Accepts), plus the drawer's "required": a
// param with no default and nothing typed.
func checkParams(r *http.Request, def wf.Workflow, given map[string]string) map[string]string {
	fields := map[string]string{}
	declared := map[string]bool{}
	for _, p := range def.Params {
		declared[p.Name] = true
		value, ok := given[p.Name]
		if !ok {
			value = p.Default
		}
		if value == "" && p.Default == "" && !p.Type.IsList() {
			fields[p.Name] = i18n.T(r.Context(), "drawer.check.required")
		} else if err := p.Accepts(value); err != nil {
			fields[p.Name] = err.Error()
		}
	}
	for name := range given {
		if !declared[name] {
			fields[name] = fmt.Sprintf("workflow %q does not declare the param %q", def.Slug, name)
		}
	}
	return fields
}

func refuse(w http.ResponseWriter, msg string, fields map[string]string) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": msg, "fields": fields})
}

// triggerJSON runs a workflow now, or one of its past slots: handler
// `disparar`'s effect (the same slot reading, the same scheduler call) answered
// as JSON. 201 {runId, href}; 422 {error, fields} for a value or slot the
// operator can fix; 409 when the idempotency key folded this click into one
// already made.
func (u *UI) triggerJSON(w http.ResponseWriter, r *http.Request) {
	ctx := i18n.WithLang(r.Context(), i18n.Negotiate(r))
	r = r.WithContext(ctx)
	slug := r.PathValue("slug")
	def, err := u.defs.Definition(ctx, slug)
	if err != nil {
		jsonError(w, http.StatusNotFound, "no such workflow")
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTriggerBody))
	dec.UseNumber()
	var body triggerBody
	// An empty body is a run with the defaults, as an empty form is.
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var fromRun uuid.UUID
	if body.FromRun != "" {
		if fromRun, err = uuid.Parse(body.FromRun); err != nil {
			jsonError(w, http.StatusBadRequest, "from_run is not a run id")
			return
		}
	}

	params, fields := map[string]string{}, map[string]string{}
	for name, v := range body.Params {
		s, err := paramString(v)
		if err != nil {
			fields[name] = err.Error()
			continue
		}
		params[name] = s
	}
	for name, msg := range checkParams(r, def, params) {
		if _, bad := fields[name]; !bad {
			fields[name] = msg
		}
	}
	slot, err := u.slotFromForm(ctx, slug, strings.TrimSpace(body.Slot))
	if err != nil {
		fields["slot"] = err.Error()
	}
	if len(fields) > 0 {
		refuse(w, i18n.N(ctx, "drawer.status.invalid", len(fields), "n", len(fields)), fields)
		return
	}

	id, err := u.actions.Disparar(ctx, slug, time.Now(), params, slot)
	if err != nil {
		u.log.Warn("trigger refused", "workflow", slug, "error", err)
		// What is left is the scheduler refusing the slot (not on the cron, in
		// the future): its sentence names the nearest valid one.
		if slot != nil {
			refuse(w, err.Error(), map[string]string{"slot": err.Error()})
			return
		}
		refuse(w, err.Error(), map[string]string{})
		return
	}
	if id == uuid.Nil {
		u.log.Info("trigger ignored by idempotency", "workflow", slug)
		jsonError(w, http.StatusConflict, "a run of this workflow was just started; follow it in Runs")
		return
	}
	u.log.Info("manual run created", "workflow", slug, "run", id, "from_run", fromRun)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"runId": id.String(), "href": "/runs/" + id.String()})
}

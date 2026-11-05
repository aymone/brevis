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

	sch "brevis.sh/brevis/internal/domain/schedule"
	wf "brevis.sh/brevis/internal/domain/workflow"
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
// with the old form's boxes. A refusal is a *wf.ParamError.
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
				return "", &wf.ParamError{Code: wf.ParamNestedList}
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
	return "", &wf.ParamError{Code: wf.ParamBadValueType}
}

// slotParseError is a slot that is not a time: Value is what was sent.
type slotParseError struct{ Value string }

func (e *slotParseError) Error() string {
	return fmt.Sprintf("slot %q is not a time: use 2026-10-27T04:00:00Z, or 2026-10-27T04:00 in the schedule's zone", e.Value)
}

// fieldCode is one field's refusal on the wire: {"code", ...data}, the data only when it exists. The
// SPA writes the sentence (web/spa/src/api/trigger.ts); the English of Error() is for the CLI and the logs.
func fieldCode(err error) map[string]any {
	var pe *wf.ParamError
	var se *sch.SlotError
	var pse *slotParseError
	switch {
	case errors.As(err, &pe):
		f := map[string]any{"code": string(pe.Code)}
		switch pe.Code {
		case wf.ParamNotBoolean, wf.ParamNotInteger, wf.ParamNotOneOf, wf.ParamNoMatch, wf.ParamUnsafeChars, wf.ParamItemDuplicate, wf.ParamUndeclared:
			f["value"] = pe.Value
		}
		if pe.Item > 0 {
			f["item"] = pe.Item
		}
		if pe.Code == wf.ParamNotOneOf {
			f["allowed"] = pe.Allowed
		}
		if pe.Code == wf.ParamNoMatch {
			f["pattern"] = pe.Pattern
		}
		return f
	case errors.As(err, &se):
		f := map[string]any{"code": string(se.Code)}
		if se.Code != sch.SlotNoSchedule {
			f["slot"] = se.Slot.UTC().Format(time.RFC3339)
		}
		if se.Code == sch.SlotNotOnCron {
			f["cron"], f["zone"] = se.Cron, se.Zone
			if !se.Nearest.IsZero() {
				f["nearest"] = se.Nearest.UTC().Format(time.RFC3339)
			}
		}
		return f
	case errors.As(err, &pse):
		return map[string]any{"code": "slot_not_a_time", "value": pse.Value}
	}
	return map[string]any{"code": "invalid"}
}

// checkParams is the trigger form's validation, field by field: what
// Workflow.Resolver refuses as one sentence, said where it belongs. The rules
// are the definition's own (Param.Accepts), plus the drawer's "required": a
// param with no default and nothing typed.
func checkParams(def wf.Workflow, given map[string]string) map[string]map[string]any {
	fields := map[string]map[string]any{}
	declared := map[string]bool{}
	for _, p := range def.Params {
		declared[p.Name] = true
		value, ok := given[p.Name]
		if !ok {
			value = p.Default
		}
		if value == "" && p.Default == "" && !p.Type.IsList() {
			fields[p.Name] = fieldCode(&wf.ParamError{Code: wf.ParamRequired})
		} else if err := p.Accepts(value); err != nil {
			fields[p.Name] = fieldCode(err)
		}
	}
	for name := range given {
		if !declared[name] {
			fields[name] = fieldCode(&wf.ParamError{Code: wf.ParamUndeclared, Value: name})
		}
	}
	return fields
}

// triggerJSON runs a workflow now, or one of its past slots: handler
// `disparar`'s effect (the same slot reading, the same scheduler call) answered
// as JSON. 201 {runId, href}; 422 {error: "invalid_params", fields} for a value
// or slot the operator can fix; 409 when the idempotency key folded this click
// into one already made.
func (u *UI) triggerJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	def, err := u.defs.Definition(ctx, slug)
	if err != nil {
		fail(w, http.StatusNotFound, codeNotFound)
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTriggerBody))
	dec.UseNumber()
	var body triggerBody
	// An empty body is a run with the defaults, as an empty form is.
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, codeInvalidRequest)
		return
	}
	var fromRun uuid.UUID
	if body.FromRun != "" {
		if fromRun, err = uuid.Parse(body.FromRun); err != nil {
			fail(w, http.StatusBadRequest, codeInvalidRequest)
			return
		}
	}

	params, fields := map[string]string{}, map[string]map[string]any{}
	for name, v := range body.Params {
		s, err := paramString(v)
		if err != nil {
			fields[name] = fieldCode(err)
			continue
		}
		params[name] = s
	}
	for name, f := range checkParams(def, params) {
		if _, bad := fields[name]; !bad {
			fields[name] = f
		}
	}
	slot, err := u.slotFromForm(ctx, slug, strings.TrimSpace(body.Slot))
	if err != nil {
		var pse *slotParseError
		if !errors.As(err, &pse) {
			u.log.Warn("trigger: reading the schedule failed", "workflow", slug, "error", err)
			fail(w, http.StatusInternalServerError, codeUnavailable)
			return
		}
		fields["slot"] = fieldCode(err)
	}
	if len(fields) > 0 {
		fail(w, http.StatusUnprocessableEntity, codeInvalidParams, "fields", fields)
		return
	}

	id, err := u.actions.Trigger(ctx, slug, time.Now(), params, slot)
	if err != nil {
		u.log.Warn("trigger refused", "workflow", slug, "error", err)
		// The scheduler refusing the slot (not on the cron, in the future) is the slot field's; any other
		// failure is the server's, and its text stays in the log.
		var se *sch.SlotError
		if errors.As(err, &se) {
			fail(w, http.StatusUnprocessableEntity, codeInvalidParams, "fields", map[string]any{"slot": fieldCode(err)})
			return
		}
		fail(w, http.StatusInternalServerError, codeUnavailable)
		return
	}
	if id == uuid.Nil {
		u.log.Info("trigger ignored by idempotency", "workflow", slug)
		fail(w, http.StatusConflict, codeRunJustStarted)
		return
	}
	u.log.Info("manual run created", "workflow", slug, "run", id, "from_run", fromRun)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"runId": id.String(), "href": "/runs/" + id.String()})
}

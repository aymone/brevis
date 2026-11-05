package api

import (
	"net/http"

	"brevis.sh/brevis/internal/auth"
)

// apiCode is the machine-readable reason of an API failure, answered as {"error": "<code>", ...}.
//
// THE SERVER SPEAKS CODES, NEVER SENTENCES. The sentence is the screen's, in the reader's language:
// web/spa/src/api/errors.ts maps each code to a catalog key, and a code it does not know reads as a
// plain failure. Add a code here and there together. What a failure may carry beside the code is
// data (a number, a name, retryAfter), never prose; the one exception is `detail`, which is the
// words of a service downstream (the warehouse, the SQL API), shown as it came.
type apiCode string

const (
	// The session endpoints (login_api.go).
	codeInvalidCredentials apiCode = "invalid_credentials" // #nosec G101 -- the name of an error code, not a credential; a wrong user and a wrong password alike
	codeTooManyAttempts    apiCode = "too_many_attempts"   // locked, with retryAfter
	codeMalformedRequest   apiCode = "malformed_request"
	codeCrossSiteRefused   apiCode = "cross_site_refused"
	codeJSONRequired       apiCode = "json_required"

	// Any endpoint.
	codeSignInRequired apiCode = auth.CodeSignInRequired // the gate's 401 (internal/auth), the SPA's cue to go to the login
	codeUnavailable    apiCode = "unavailable"           // a read failed; trying again may work
	codeNotFound       apiCode = "not_found"             // what the path names does not exist
	codeNotEnabled     apiCode = "not_enabled"           // the console runs without that service
	codeInvalidRequest apiCode = "invalid_request"       // a body, a parameter or a content type the endpoint refuses
	codeForbidden      apiCode = "forbidden"

	// Particular to one action.
	codeInvalidParams      apiCode = "invalid_params"        // POST trigger: 422 with `fields`
	codeRunJustStarted     apiCode = "run_just_started"      // POST trigger: the idempotency key folded the click into a run
	codeCancelFailed       apiCode = "cancel_failed"         // POST cancel
	codeScheduleFailed     apiCode = "schedule_failed"       // POST toggle
	codeNoSchedule         apiCode = "no_schedule"           // POST toggle: a workflow with none
	codeNotWritten         apiCode = "not_written_by_brevis" // a destination no writer of Brevis lands in
	codeUnknownDestination apiCode = "unknown_destination"   // the SQL workbench's target is not in the catalog
	codeSQLNotConfigured   apiCode = "sql_not_configured"
	codeStatementTooLong   apiCode = "statement_too_long"
	codeStatementEmpty     apiCode = "statement_empty"
	codeNoConnection       apiCode = "no_connection"  // with `connection`
	codeRefused            apiCode = "refused"        // the SQL API refused the statement; `detail` is its reason
	codeRunFailed          apiCode = "run_failed"     // the warehouse or the endpoint failed; `detail` is its message
	codeInvalidTopics      apiCode = "invalid_topics" // GET /api/events
	codeBusy               apiCode = "busy"           // GET /api/events: no room for another stream

	// Try it on an endpoint (POST .../call): why a call brought nothing back. A failure of none of
	// these kinds is codeRunFailed with the error's own text as `detail`.
	codeEndpointNotConfigured apiCode = "endpoint_not_configured"
	codeEndpointUnbuilt       apiCode = "endpoint_unbuilt"
	codeEndpointUnreachable   apiCode = "endpoint_unreachable"
	codeEndpointUnreadable    apiCode = "endpoint_unreadable"
	codeEndpointTooLarge      apiCode = "endpoint_too_large"
	codeEndpointNotAnswer     apiCode = "endpoint_not_answer"
)

// fail answers an API failure as its code. extra is name, value pairs of data to carry beside it.
func fail(w http.ResponseWriter, status int, code apiCode, extra ...any) {
	body := map[string]any{"error": string(code)}
	for i := 0; i+1 < len(extra); i += 2 {
		body[extra[i].(string)] = extra[i+1]
	}
	writeJSON(w, status, body)
}

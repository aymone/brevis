package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"brevis.sh/brevis/internal/auth"
)

// maxLoginBody bounds what /api/login reads. A user, a password and a path fit
// in a few hundred bytes; the cap keeps a flood of large bodies from being
// buffered before the Limiter ever sees the attempt.
const maxLoginBody = 8 << 10

// jsonOnly and sameOrigin are the CSRF gate of the two session endpoints. They
// are two independent checks on purpose, and each one alone stops the classic
// attack (a hidden <form> on another site posting to /api/login, which would
// log the victim's browser into the ATTACKER's account or probe passwords from
// the victim's address):
//
//   - A cross-site HTML form can only send urlencoded, multipart or text/plain.
//     application/json needs fetch(), and a cross-origin fetch with that type
//     triggers a CORS preflight this server never approves.
//   - The browser states where the request came from (Sec-Fetch-Site, or
//     Origin on engines without it). A request that says it is not from this
//     very origin is refused.
//
// A client with neither header (curl, a script) passes the second check: only
// a browser can be tricked by a page, and the password is still required.
func jsonOnly(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

func sameOrigin(r *http.Request) bool {
	// Sec-Fetch-Site first, as Server.crossSite does: a browser sets it itself
	// (a page cannot forge a Sec-* header), and it stays truthful behind a
	// proxy that rewrites Host, where comparing Origin to Host would refuse the
	// operator's own tab (the Vite dev proxy does exactly that). "same-site"
	// (a sibling subdomain) is refused: nothing but this origin should post here.
	switch r.Header.Get("Sec-Fetch-Site") {
	case "":
	case "same-origin", "none":
		return true
	default:
		return false
	}
	// No Sec-Fetch-Site (older browsers, scripts): Origin decides when present.
	// "null" (a sandboxed frame, a redirect chain) parses to an empty host and
	// is refused with every other foreign origin.
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		return err == nil && u.Host != "" && u.Host == r.Host
	}
	return true
}

// guardSession answers the refusal for a request that fails either check, and
// reports whether the handler may go on.
func guardSession(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request refused"})
		return false
	}
	if !jsonOnly(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "send application/json"})
		return false
	}
	return true
}

type loginRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Next     string `json:"next"`
}

// RegistrarLoginAPI wires the SPA's session endpoints. They exist only when there is a credential: an open console has no login to
// answer, and these routes would then be a second door nobody configured.
func (u *UI) RegistrarLoginAPI(mux *http.ServeMux, gate *auth.Gate) {
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		if !guardSession(w, r) {
			return
		}
		var req loginRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed request"})
			return
		}
		// Exactly one JSON value: trailing bytes are a malformed body, not
		// something to ignore.
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed request"})
			return
		}

		// The guard against an open redirect: no `next` means the front page.
		next := "/"
		if req.Next != "" {
			next = auth.Target(req.Next)
		}

		// Keyed by the connection's address, or the header the deployment's
		// proxy sets.
		origin := auth.ClientKey(r, gate.Cred.ClientIPHeader)

		out, retry := gate.Attempt(r.Context(), w, origin, req.User, req.Password)
		switch out {
		case auth.Locked, auth.Busy:
			// Attempt ran no hash for these. The password the client sent while
			// locked is not examined, so a correct one is refused too.
			secs := int(retry.Seconds()) + 1
			u.log.Warn("login throttled", "origin", origin)
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error":      "Too many attempts. Try again in a moment.",
				"retryAfter": secs,
			})
		case auth.Signed:
			u.log.Info("login", "user", req.User)
			writeJSON(w, http.StatusOK, map[string]string{"next": next})
		default:
			// One sentence for a wrong user and a wrong password alike; neither
			// the username nor the password enters the log (a password typed
			// in the wrong box would land there).
			u.log.Warn("login refused", "origin", origin)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid username or password."})
		}
	})

	// POST, and under the same CSRF gate: an <img src> must not sign anyone out.
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		if !guardSession(w, r) {
			return
		}
		gate.SignOut(w)
		w.WriteHeader(http.StatusNoContent)
	})
}

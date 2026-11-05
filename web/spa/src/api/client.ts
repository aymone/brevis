// ApiError carries the HTTP status, so a screen can tell a 401 (go to login)
// from a 500 (show the retry) without parsing a message.
//
// The server speaks codes, never sentences (internal/api/apierror.go): `code` is its
// {"error"} and `body` the rest of it (data such as `connection` or `retryAfter`, and
// `detail`, the words of a service downstream). The sentence is the screen's, in the
// reader's language: see errors.ts. `message` is the code, or "<path>: HTTP <status>"
// when the server said nothing, and is for logs, never for a reader.
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    /** The parsed JSON error body ({"error": "<code>", ...data}), when the server sent one. */
    readonly body?: Record<string, unknown>,
  ) {
    super(message);
  }
  /** The server's error code, "" when it sent none. */
  get code(): string {
    return typeof this.body?.error === "string" ? this.body.error : "";
  }
}

// The Gate answers an /api/* request without a session with 401 {"error":"sign_in_required"}
// (a redirect is for pages). The way back is the app's own login screen, which
// returns here afterwards through `next`. replace(), not assign(): the screen
// that lost its session should not be one Back away. Only the Gate's own body
// counts: a 401 some endpoint relays from elsewhere is that screen's error to
// show, not a reason to drop the operator's session view. On the login screen
// itself there is nowhere to go: the caller (boot) handles the 401.
async function signInAgain(res: Response, path: string): Promise<void> {
  if (res.status !== 401) return;
  const body = (await res.clone().json().catch(() => null)) as { error?: string } | null;
  if (body?.error !== "sign_in_required") return;
  if (location.pathname !== "/login") {
    location.replace(`/login?next=${encodeURIComponent(location.pathname + location.search)}`);
  }
  throw new ApiError(401, `${path}: no session`);
}

// A read takes no AbortSignal, and the query functions must not even READ the one
// React Query offers (`({ signal }) =>`): merely touching it tells the library the
// request is cancellable, so when the last observer leaves it cancels and starts
// over on the next mount. StrictMode (dev) mounts every screen twice and so sent
// every request twice; a screen that is left mid-flight listed a red "(canceled)".
// These reads are small and land in the cache, so they simply finish and the next
// observer joins the request in flight. Where a request really is the reader's to
// cancel -- the SQL run -- postJSON still honors a signal.
export async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { Accept: "application/json" } });
  await signInAgain(res, path);
  if (!res.ok) {
    // The API answers a failure as {"error": "<code>", ...}; say that when it did.
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    const code = typeof body?.error === "string" ? body.error : "";
    throw new ApiError(res.status, code || `${path}: HTTP ${res.status}`, body ?? undefined);
  }
  // A 204 is "nothing there" (no release known): no body to read.
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/** POST a JSON body; resolves undefined on 204. A failure is an ApiError with the server's {"error"} code. */
export async function postJSON<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T | undefined> {
  const res = await fetch(path, {
    method: "POST",
    headers: { Accept: "application/json", ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });
  await signInAgain(res, path);
  if (res.status === 204) return undefined;
  if (!res.ok) {
    const parsed = (await res.json().catch(() => undefined)) as Record<string, unknown> | undefined;
    const code = typeof parsed?.error === "string" ? parsed.error : "";
    throw new ApiError(res.status, code || `${path}: HTTP ${res.status}`, parsed);
  }
  return (await res.json()) as T;
}

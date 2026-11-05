// The sign-in and sign-out calls. They do NOT use client.ts's getJSON/postJSON:
// those turn a 401 into "go to the login", and here a 401 is the answer to a
// wrong password, which the login screen shows in place.
export type SignIn =
  | { ok: true; next: string }
  | { ok: false; status: number; error: string; retryAfter?: number };

// An empty `error` means the server said nothing: the screen writes its own, in the reader's language.
// The password travels once, in this request's body, and is not kept anywhere
// here. Content-Type is application/json on purpose: the server accepts nothing
// else, because a cross-site HTML form cannot send it (CSRF).
export async function signIn(user: string, password: string, next: string): Promise<SignIn> {
  try {
    const res = await fetch("/api/login", {
      method: "POST",
      headers: { Accept: "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ user, password, next }),
    });
    const body = (await res.json().catch(() => ({}))) as { next?: string; error?: string; retryAfter?: number };
    if (res.ok) return { ok: true, next: body.next ?? "/" };
    return { ok: false, status: res.status, error: body.error ?? "", retryAfter: body.retryAfter };
  } catch {
    return { ok: false, status: 0, error: "" };
  }
}

/** Where a post-login destination may go: a same-origin path, never `//host` or a backslash (the server's guard, again). */
export function safeNext(raw: string | null | undefined): string {
  if (!raw || !/^\/(?![/\\])/.test(raw) || /[\u0000-\u001f\u007f]/.test(raw)) return "/";
  // A bookmark from when the console lived under /app/ still lands on the right screen.
  return raw.replace(/^\/app(?=[/?#]|$)/, "") || "/";
}

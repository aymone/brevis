import type { QueryClient } from "@tanstack/react-query";
import { ApiError, getJSON } from "./api/client";
import type { Config, Session } from "./api/session";
import { bindOperator } from "./ui/seen";

type Messages = { lang: string; messages: Record<string, unknown> };

// The session is the one of the three that needs a login. Without one the Gate
// answers 401 and the app is on its login screen (/login) with the visitor
// marked anonymous; anywhere else the client has already started the trip to
// that screen, and there is nothing to draw in the meantime.
async function readSession(): Promise<Session | undefined> {
  try {
    return await getJSON<Session>("/api/session");
  } catch (e) {
    if (!(e instanceof ApiError) || e.status !== 401) throw e;
    if (location.pathname !== "/login") return new Promise<never>(() => {});
    return undefined;
  }
}

// WHAT IS STATIC MUST NOT WAIT. The menu, the titles and the labels are words
// from the catalog, and the brand and the session are known before any screen
// asks for data. So the three are read once, in parallel, BEFORE the first
// render and put in the query cache: from then on useT(), useSession() and
// useConfig() answer at once and never suspend. A skeleton is for what a screen
// loads (rows, numbers, charts), never for a heading or a link.
// Config and messages are public (the login screen needs both); the session is not.
export async function boot(client: QueryClient): Promise<void> {
  const [session, config, messages] = await Promise.all([
    readSession(),
    getJSON<Config>("/api/config"),
    getJSON<Messages>("/api/messages"),
    // The faces the screens use, fetched alongside: font-display is swap, so a face that lands after
    // the first paint re-wraps every line (a whole screen's text moves). Capped, so a slow font
    // costs a short wait, not a blank page.
    Promise.race([
      Promise.all(['400 1em "IBM Plex Sans"', '500 1em "IBM Plex Sans"', '600 1em "IBM Plex Sans"', '400 1em "IBM Plex Mono"', '500 1em "IBM Plex Mono"'].map((f) => document.fonts.load(f))),
      new Promise((done) => setTimeout(done, 400)),
    ]).catch(() => undefined),
  ]);
  // The client's theme goes in BEFORE the first render: applied from an effect it
  // would paint one frame in the default colours first.
  if (config.brand.css) {
    const el = document.createElement("style");
    el.dataset.brand = "";
    el.textContent = config.brand.css;
    document.head.appendChild(el);
  }
  // index.html says "en"; the visitor's language is what screen readers and hyphenation follow.
  document.documentElement.lang = messages.lang;
  // No session: a client-side placeholder (the server never sends `anonymous`), so
  // useT() has a language and the login screen can tell a visitor from an open console.
  const who: Session = session ?? { operator: "", lang: messages.lang, anonymous: true };
  if (session) bindOperator(session.operator);
  client.setQueryData(["session"], who);
  client.setQueryData(["config"], config);
  client.setQueryData(["messages", who.lang], messages);
}

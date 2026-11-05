import { useSuspenseQuery } from "@tanstack/react-query";
import { getJSON } from "./client";

export type Session = {
  operator: string;
  lang: string;
  /** Set by boot (never by the server) when there was no session: the visitor is on the login screen. */
  anonymous?: true;
};
export type Brand = {
  title: string;
  subtitle: string;
  phrase: string;
  logo: string;
  attribution: string;
  css: string;
};
export type Config = { brand: Brand; version: string; sql: boolean };

// Both are read once per visit: who is looking and which installation this is
// do not change while a tab is open. The session is per person (never cached by
// the server); the config is the installation's. A missing session is handled where the
// client sees it (client.ts), not here.
const forever = { staleTime: Infinity, gcTime: Infinity } as const;

export const useSession = () =>
  useSuspenseQuery({ queryKey: ["session"], queryFn: () => getJSON<Session>("/api/session"), ...forever }).data;

export const useConfig = () =>
  useSuspenseQuery({ queryKey: ["config"], queryFn: () => getJSON<Config>("/api/config"), ...forever }).data;

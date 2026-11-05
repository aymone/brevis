import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";

/**
 * The search text of a list whose address carries it (`q`, `workflow`...). What is typed stays local, so
 * typing never remounts anything; the address moves ONE beat (250ms) after the last key, replacing the
 * history entry (typing does not fill the back button) and dropping `page`. An outside change (a link,
 * Back, a chip) replaces what is typed. Same parameter, same URL as before: links stay shareable.
 */
export function useUrlSearch(param = "q") {
  const [sp] = useSearchParams();
  const navigate = useNavigate();
  const url = sp.get(param) ?? "";
  const [text, setText] = useState(url);
  const sent = useRef(url);
  useEffect(() => {
    if (url !== sent.current) {
      sent.current = url;
      setText(url);
    }
  }, [url]);
  useEffect(() => {
    const v = text.trim();
    if (v === sent.current) return;
    const id = setTimeout(() => {
      sent.current = v;
      const q = new URLSearchParams(window.location.search);
      q.delete("page");
      if (v) q.set(param, v);
      else q.delete(param);
      void navigate(`?${q.toString()}`, { replace: true });
    }, 250);
    return () => clearTimeout(id);
  }, [text, navigate, param]);
  return [text, setText] as const;
}

import { Component, type ReactNode } from "react";
import { QueryErrorResetBoundary } from "@tanstack/react-query";
import { useT } from "../i18n";
import { buttonClass } from "./button";

type State = { failed: boolean };

/** What a region says when its data would not load, with the way to ask again. */
export function LoadError({ onRetry }: { onRetry: () => void }) {
  const { t } = useT();
  return (
    <div role="alert" className="flex flex-col items-start gap-2 rounded-card border border-dashed border-line px-4 py-3 text-sm text-muted">
      <span>{t("loaderror.text")}</span>
      <button type="button" onClick={onRetry} className={buttonClass("secondary", "small")}>{t("loaderror.retry")}</button>
    </div>
  );
}

class Boundary extends Component<{ onReset: () => void; children: ReactNode }, State> {
  state: State = { failed: false };
  static getDerivedStateFromError(): State {
    return { failed: true };
  }
  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <LoadError
        onRetry={() => {
          this.props.onReset();
          this.setState({ failed: false });
        }}
      />
    );
  }
}

/** Catches a failed query below it and offers to ask again; the rest of the screen stays. */
export function QueryBoundary({ children }: { children: ReactNode }) {
  return <QueryErrorResetBoundary>{({ reset }) => <Boundary onReset={reset}>{children}</Boundary>}</QueryErrorResetBoundary>;
}

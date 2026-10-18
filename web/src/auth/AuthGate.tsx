import { AlertCircle } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router";

import { type AuthSession, getAuthSession } from "../api/auth";
import { RequestFailed, setRenderedAudience } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { isVideoEditPath } from "../lib/videoEditPath";
import { Button } from "../ui/shadcn/button";
import { EmptyState } from "../videoList/states";
import { AudienceProvider } from "./audience";
import { currentPath, loginPath, reloadPage } from "./pageNavigation";

type GateState =
  | { status: "checking" }
  | { status: "failed"; reason: UiText }
  // The /login query used by the session check, including whether next was supplied.
  | { status: "ready"; session: AuthSession; search: string | null };

/**
 * Match React Router's case and trailing-slash normalization so paths such as /Settings cannot
 * bypass the gate.
 */
export function routePath(pathname: string): string {
  const lowered = pathname.toLowerCase().replace(/\/+$/, "");
  return lowered === "" ? "/" : lowered;
}

/** Expects a pathname normalized by routePath. */
function ownerOnlyPath(pathname: string): boolean {
  return (
    [
      "/settings",
      "/tags",
      "/people",
      "/organizations",
      "/history",
      "/playlists",
      "/duplicates",
      "/activity",
    ].some((path) => pathname === path || pathname.startsWith(`${path}/`)) ||
    isVideoEditPath(pathname)
  );
}

function loginNext(search: string): string | undefined {
  return new URLSearchParams(search).get("next") ?? undefined;
}

function failureReason(error: unknown): UiText {
  if (error instanceof RequestFailed) return errorText(error);
  return t.auth.gate.noResponse;
}

/** Use the server-validated redirectTo; failed SPA rechecks require an explicit retry. */
function OwnerLoginRedirect({
  session,
  checkedSearch,
}: {
  session: AuthSession;
  checkedSearch: string | null;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const [attempt, setAttempt] = useState(0);
  const [failure, setFailure] = useState<UiText | null>(null);

  useEffect(() => {
    if (checkedSearch === location.search) {
      void navigate(session.redirectTo ?? "/", { replace: true });
      return;
    }
    // SPA navigation may supply a different next value from the gate's session check.
    const controller = new AbortController();
    getAuthSession(loginNext(location.search), controller.signal).then(
      (current) => {
        if (current.state !== "owner") {
          reloadPage();
          return;
        }
        void navigate(current.redirectTo ?? "/", { replace: true });
      },
      (error: unknown) => {
        if (controller.signal.aborted) return;
        setFailure(failureReason(error));
      },
    );
    return () => controller.abort();
  }, [session, checkedSearch, location.search, navigate, attempt]);

  if (failure === null) return null;
  return (
    <GateFailure
      reason={failure}
      onRetry={() => {
        setFailure(null);
        setAttempt((current) => current + 1);
      }}
    />
  );
}

function GateFailure({ reason, onRetry }: { reason: UiText; onRetry: () => void }) {
  return (
    <div className="flex min-h-dvh items-center bg-background">
      <EmptyState
        icon={AlertCircle}
        tone="danger"
        title={t.auth.gate.unreachable}
        description={reason}
        action={
          <Button size="sm" onClick={onRetry}>
            {t.common.retry}
          </Button>
        }
      />
    </div>
  );
}

/**
 * Render nothing until the session is known; route setup, guests and owners accordingly. Failed
 * checks require an explicit retry.
 */
export default function AuthGate({ children }: { children: ReactNode }) {
  const location = useLocation();
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<GateState>({ status: "checking" });

  // Check only on initial load or Retry; audience changes reload the page.
  const [initialLocation] = useState(location);
  useEffect(() => {
    const controller = new AbortController();
    const onLogin = routePath(initialLocation.pathname) === "/login";
    const next = onLogin ? loginNext(initialLocation.search) : undefined;
    getAuthSession(next, controller.signal).then(
      (session) => {
        // client.ts reloads once if the audience changes while rendering as owner.
        setRenderedAudience(
          session.state === "owner" || session.state === "guest" ? session.state : null,
        );
        setState({
          status: "ready",
          session,
          search: onLogin ? initialLocation.search : null,
        });
      },
      (error: unknown) => {
        if (controller.signal.aborted) return;
        setState({ status: "failed", reason: failureReason(error) });
      },
    );
    return () => controller.abort();
  }, [attempt, initialLocation]);

  if (state.status === "checking") return null;
  if (state.status === "failed") {
    return (
      <GateFailure
        reason={state.reason}
        onRetry={() => {
          setState({ status: "checking" });
          setAttempt((current) => current + 1);
        }}
      />
    );
  }

  const { session } = state;
  const pathname = routePath(location.pathname);

  if (session.state === "setupRequired") {
    if (pathname !== "/setup") return <Navigate replace to="/setup" />;
    return children;
  }

  if (pathname === "/setup") return <Navigate replace to="/" />;

  if (session.state === "guest") {
    if (ownerOnlyPath(pathname)) {
      return <Navigate replace to={loginPath(currentPath(location))} />;
    }
    return <AudienceProvider audience="guest">{children}</AudienceProvider>;
  }

  if (pathname === "/login") {
    return <OwnerLoginRedirect session={session} checkedSearch={state.search} />;
  }
  return <AudienceProvider audience="owner">{children}</AudienceProvider>;
}

import { AlertCircle } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router";

import { type AuthSession, getAuthSession } from "../api/auth";
import { RequestFailed } from "../api/client";
import Button from "../ui/Button";
import { EmptyState } from "../videoList/states";
import { AudienceProvider } from "./audience";
import { currentPath, loginPath, reloadPage } from "./pageNavigation";

type GateState =
  | { status: "checking" }
  | { status: "failed"; reason: string }
  // search は、確かめたときの /login の問い合わせ（next を送ったか）である。
  | { status: "ready"; session: AuthSession; search: string | null };

/** ownerOnlyPath は所有者だけの画面（設定・タグの管理）かを返す。 */
function ownerOnlyPath(pathname: string): boolean {
  return ["/settings", "/tags"].some(
    (path) => pathname === path || pathname.startsWith(`${path}/`),
  );
}

/** loginNext は /login の問い合わせの next を返す。無ければ undefined。 */
function loginNext(search: string): string | undefined {
  return new URLSearchParams(search).get("next") ?? undefined;
}

function failureReason(error: unknown): string {
  if (error instanceof RequestFailed) return error.message;
  return "サーバーから応答がありません。サーバーが動いているか確かめてください";
}

/**
 * OwnerLoginRedirect は、ログイン済みで /login を開いたとき、サーバーが確かめた
 * 戻り先（redirectTo）へ置き換える。戻り先は画面で判定しない
 * （specs/016-single-account-auth/contracts/auth-api.md §4）。
 */
function OwnerLoginRedirect({
  session,
  checkedSearch,
}: {
  session: AuthSession;
  checkedSearch: string | null;
}) {
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => {
    if (checkedSearch === location.search) {
      void navigate(session.redirectTo ?? "/", { replace: true });
      return;
    }
    // SPA の中で /login へ来たとき。ゲートが確かめたときとは next が違う。
    const controller = new AbortController();
    getAuthSession(loginNext(location.search), controller.signal).then(
      (current) => {
        if (current.state !== "owner") {
          reloadPage();
          return;
        }
        void navigate(current.redirectTo ?? "/", { replace: true });
      },
      () => {
        if (!controller.signal.aborted) void navigate("/", { replace: true });
      },
    );
    return () => controller.abort();
  }, [session, checkedSearch, location.search, navigate]);

  return null;
}

function GateFailure({ reason, onRetry }: { reason: string; onRetry: () => void }) {
  return (
    <div className="flex min-h-dvh items-center bg-bg">
      <EmptyState
        icon={AlertCircle}
        tone="danger"
        title="サーバーに接続できません"
        description={reason}
        action={<Button onClick={onRetry}>再試行</Button>}
      />
    </div>
  );
}

/**
 * AuthGate は見る人の状態（GET /api/auth/session）が分かるまで何も描かず、分かったら
 * 状態に合う画面へ振り分ける（specs/016-single-account-auth/ui-design.md「Gate」）。
 *
 * - setupRequired: どの URL でも /setup
 * - guest: 所有者だけの画面は /login?next=…、/setup は /
 * - owner: /login はサーバーの redirectTo、/setup は /
 *
 * 確認に失敗したら何も描かずに理由と「再試行」を出す。自動では確かめ直さない。
 */
export default function AuthGate({ children }: { children: ReactNode }) {
  const location = useLocation();
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<GateState>({ status: "checking" });

  // 確かめるのは読み込みのときと「再試行」のときだけである。遷移のたびには確かめない。
  // 見る人が変わるときは、ページごと読み直してここからやり直す。
  const [initialLocation] = useState(location);
  useEffect(() => {
    const controller = new AbortController();
    const onLogin = initialLocation.pathname === "/login";
    const next = onLogin ? loginNext(initialLocation.search) : undefined;
    getAuthSession(next, controller.signal).then(
      (session) =>
        setState({
          status: "ready",
          session,
          search: onLogin ? initialLocation.search : null,
        }),
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
  const { pathname } = location;

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

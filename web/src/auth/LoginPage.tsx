import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";

import { login, LoginThrottled } from "../api/auth";
import { RequestFailed } from "../api/client";
import Button from "../ui/Button";
import {
  connectionWarningId,
  ConnectionWarning,
  CredentialField,
  CredentialScreen,
  FailureLine,
  UsernameField,
} from "./CredentialScreen";
import { assignPage } from "./pageNavigation";

/** loginFailureMessage は、ログインの失敗を失敗の行の文言にする（ui-design.md「Login behaviour」）。 */
function loginFailureMessage(error: unknown): string {
  if (error instanceof LoginThrottled) {
    return error.retryAfterSeconds === null
      ? "試行が多すぎます。しばらくしてからやり直してください"
      : `試行が多すぎます。${error.retryAfterSeconds} 秒後にやり直してください`;
  }
  if (error instanceof RequestFailed) {
    // どの欄が違うかは示さない（要件 12）。
    if (error.status === 401) return "ユーザー名またはパスワードが違います";
    return "ログインできませんでした。もう一度お試しください";
  }
  return "サーバーに接続できません";
}

/**
 * LoginPage はログイン画面（/login）である。成功したら、サーバーが確かめた戻り先へ
 * ページごと移る。戻り先の候補は URL の next で、画面では判定しない。
 */
export default function LoginPage() {
  const location = useLocation();
  const next = new URLSearchParams(location.search).get("next") ?? undefined;

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [throttled, setThrottled] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  // Enter の二重送信を、描き直しを待たずに止める。
  const busy = useRef(false);
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const throttleTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    usernameRef.current?.focus();
    return () => window.clearTimeout(throttleTimer.current);
  }, []);

  const submit = async () => {
    if (busy.current) return;
    busy.current = true;
    setSubmitting(true);
    try {
      const { redirectTo } = await login(username, password, next);
      // ページが移るまで主操作は押せないままにする。
      assignPage(redirectTo);
      return;
    } catch (error) {
      setFailure(loginFailureMessage(error));
      if (error instanceof LoginThrottled) {
        if (error.retryAfterSeconds !== null) {
          setThrottled(true);
          window.clearTimeout(throttleTimer.current);
          throttleTimer.current = window.setTimeout(() => {
            setThrottled(false);
            busy.current = false;
          }, error.retryAfterSeconds * 1000);
          setSubmitting(false);
          return;
        }
      } else if (error instanceof RequestFailed && error.status === 401) {
        // パスワードだけを空にしてそこへ移る。ユーザー名は残す。
        setPassword("");
        passwordRef.current?.focus();
      }
    }
    busy.current = false;
    setSubmitting(false);
  };

  const warning = connectionWarningId();

  return (
    <CredentialScreen title="ログイン" onSubmit={() => void submit()}>
      <div className="flex flex-col gap-4">
        <UsernameField
          ref={usernameRef}
          id="login-username"
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          aria-describedby={warning}
        />
        <CredentialField
          ref={passwordRef}
          id="login-password"
          label="パスワード"
          name="password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
      </div>
      <FailureLine message={failure} />
      <Button
        type="submit"
        variant="primary"
        size="lg"
        className="w-full"
        disabled={submitting || throttled}
        aria-describedby={warning}
      >
        {submitting && (
          <LoaderCircle
            className="animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        )}
        ログイン
      </Button>
      <ConnectionWarning />
    </CredentialScreen>
  );
}

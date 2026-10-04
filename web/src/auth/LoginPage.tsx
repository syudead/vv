import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";

import { login, LoginThrottled } from "../api/auth";
import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  connectionWarningId,
  CredentialField,
  CredentialScreen,
  FailureLine,
  UsernameField,
} from "./CredentialScreen";
import { assignPage } from "./pageNavigation";

/** loginFailureMessage は、ログインの失敗を失敗の行の文言にする（ui-design.md「Login behaviour」）。 */
function loginFailureMessage(error: unknown): UiText {
  if (error instanceof LoginThrottled && error.retryAfterSeconds !== null) {
    return t.auth.login.throttledFor(error.retryAfterSeconds);
  }
  // どの欄が違うかは示さない（要件 12）。本文を読めない 401 も同じ文にする。
  if (error instanceof RequestFailed && error.status === 401) {
    return t.errors.code.invalid_credentials;
  }
  // 試行の制限（待ち時間なし）・その他の API エラー・届かなかった失敗は、API エラーの
  // 表示（reason → code → message → HTTP 状態）に任せる。
  return errorText(error);
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
  const [failure, setFailure] = useState<UiText | null>(null);
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
    <CredentialScreen
      title={t.auth.login.title}
      description={t.auth.login.description}
      onSubmit={() => void submit()}
      submit={
        <Button
          type="submit"
          disabled={submitting || throttled}
          aria-busy={submitting}
          aria-describedby={warning}
        >
          {submitting && (
            <LoaderCircle
              className="animate-spin motion-reduce:animate-none"
              aria-hidden="true"
            />
          )}
          {submitting ? t.auth.login.submitting : t.auth.login.submit}
        </Button>
      }
    >
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
        label={t.auth.fields.password}
        name="password"
        type="password"
        autoComplete="current-password"
        value={password}
        onChange={(event) => setPassword(event.target.value)}
      />
      <FailureLine message={failure} />
    </CredentialScreen>
  );
}

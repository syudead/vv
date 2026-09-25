import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { setupAccount } from "../api/auth";
import { RequestFailed } from "../api/client";
import Button from "../ui/Button";
import {
  connectionWarningId,
  ConnectionWarning,
  CredentialField,
  CredentialScreen,
  describedBy,
  FAILURE_ID,
  FailureLine,
  UsernameField,
} from "./CredentialScreen";
import { assignPage } from "./pageNavigation";

/** MAX_USERNAME_LENGTH はユーザー名の文字数（コードポイント数）の上限（data-model.md §6）。 */
const MAX_USERNAME_LENGTH = 128;

type SetupField = "username" | "password" | "confirm";

/**
 * usernameBreaksRule は、ユーザー名が data-model.md §6 の規則（128 文字まで、
 * 前後の空白と制御文字なし）を外れるかを返す。空かどうかは別に確かめる。
 * 正本はサーバー（internal/domain）で、ここは送る前の案内である。
 */
function usernameBreaksRule(username: string): boolean {
  return (
    Array.from(username).length > MAX_USERNAME_LENGTH ||
    /\p{Cc}/u.test(username) ||
    // 空白の判定はサーバーの unicode.IsSpace と同じ Unicode の White_Space にそろえる。
    // JavaScript の \s は U+FEFF も含み、サーバーが通す名前を止めてしまう。
    /^\p{White_Space}|\p{White_Space}$/u.test(username)
  );
}

interface Invalid {
  field: SetupField;
  message: string;
}

/** validateSetup は送る前の検証である。名指しする欄と理由を返す。 */
export function validateSetup(
  username: string,
  password: string,
  confirm: string,
): Invalid | null {
  if (username === "") {
    return { field: "username", message: "ユーザー名を入力してください" };
  }
  if (usernameBreaksRule(username)) {
    return {
      field: "username",
      message: "ユーザー名は 128 文字まで、前後の空白と制御文字なしにしてください",
    };
  }
  if (password === "") {
    return { field: "password", message: "パスワードを入力してください" };
  }
  if (confirm !== password) {
    return { field: "confirm", message: "確認用のパスワードが一致しません" };
  }
  return null;
}

/**
 * SetupPage は初回設定画面（/setup）である。最初のアカウントを作り、成功したら
 * 所有者の一覧へページごと移る（specs/016-single-account-auth/ui-design.md「Setup behaviour」）。
 */
export default function SetupPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const [invalidField, setInvalidField] = useState<SetupField | null>(null);
  const busy = useRef(false);
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const confirmRef = useRef<HTMLInputElement>(null);
  const refs = { username: usernameRef, password: passwordRef, confirm: confirmRef };
  const loginLink = useRef<HTMLAnchorElement>(null);

  // 初回だけ。ref は描き直しても同じものなので、依存に書いても一度しか動かない。
  useEffect(() => {
    usernameRef.current?.focus();
  }, [usernameRef]);

  useEffect(() => {
    if (configured) loginLink.current?.focus();
  }, [configured]);

  const submit = async () => {
    if (busy.current || configured) return;

    const invalid = validateSetup(username, password, confirm);
    if (invalid !== null) {
      setFailure(invalid.message);
      setInvalidField(invalid.field);
      if (invalid.field === "confirm") setConfirm("");
      refs[invalid.field].current?.focus();
      return;
    }

    busy.current = true;
    setSubmitting(true);
    setInvalidField(null);
    try {
      const { redirectTo } = await setupAccount(username, password);
      assignPage(redirectTo);
      return;
    } catch (error) {
      if (error instanceof RequestFailed && error.status === 409) {
        // 同時の初回設定で負けた側。入力を空にし、ログインへ進ませる。
        setFailure("アカウントは既に設定されています");
        setUsername("");
        setPassword("");
        setConfirm("");
        setConfigured(true);
      } else if (error instanceof RequestFailed && error.status === 400) {
        setFailure(error.message);
      } else if (error instanceof RequestFailed) {
        setFailure("初回設定できませんでした。もう一度お試しください");
      } else {
        setFailure("サーバーに接続できません");
      }
    }
    busy.current = false;
    setSubmitting(false);
  };

  const warning = connectionWarningId();
  const fieldProps = (field: SetupField) =>
    invalidField === field
      ? { "aria-invalid": true as const, "aria-describedby": FAILURE_ID }
      : {};

  return (
    <CredentialScreen
      title="アカウントを作成"
      description="このサーバーを使うアカウントを1つ作ります。あとから変えるにはサーバーのコマンドを使います"
      onSubmit={() => void submit()}
    >
      <div className="flex flex-col gap-4">
        <UsernameField
          ref={refs.username}
          id="setup-username"
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          {...fieldProps("username")}
          aria-describedby={describedBy(
            invalidField === "username" && FAILURE_ID,
            warning,
          )}
        />
        <CredentialField
          ref={refs.password}
          id="setup-password"
          label="パスワード"
          name="new-password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          {...fieldProps("password")}
        />
        <CredentialField
          ref={refs.confirm}
          id="setup-confirm"
          label="パスワード（確認）"
          name="confirm-password"
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={(event) => setConfirm(event.target.value)}
          {...fieldProps("confirm")}
        />
      </div>
      <FailureLine message={failure} />
      <Button
        type="submit"
        variant="primary"
        size="lg"
        className="w-full"
        disabled={submitting || configured}
        aria-describedby={warning}
      >
        {submitting && (
          <LoaderCircle
            className="animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        )}
        設定してはじめる
      </Button>
      <ConnectionWarning />
      {configured && (
        // SPA の遷移にしない。ゲートが setupRequired を覚えたままなので、
        // ページごと読み直して状態を確かめ直させる（ui-design.md「Setup behaviour」）。
        <a
          ref={loginLink}
          href="/login"
          className="self-start text-sm text-link underline underline-offset-4"
        >
          ログインへ
        </a>
      )}
    </CredentialScreen>
  );
}

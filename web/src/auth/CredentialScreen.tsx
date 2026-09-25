import { AlertCircle, ShieldAlert } from "lucide-react";
import {
  type FormEvent,
  forwardRef,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../lib/cn";

// 初回設定画面とログイン画面が共有する骨格と部品
// （specs/016-single-account-auth/ui-design.md「Credential screens」）。

/** CONNECTION_WARNING_ID は接続の警告の要素の id。入力と主操作が aria-describedby で指す。 */
export const CONNECTION_WARNING_ID = "connection-warning";
/** FAILURE_ID は失敗の行の id。初回設定の検証で名指しした欄が指す。 */
export const FAILURE_ID = "credential-failure";

/**
 * isInsecureConnection は、ページが HTTPS で開かれていないかを返す。
 * isSecureContext は http://127.0.0.1 でも真になるので使わない（要件 14）。
 */
export function isInsecureConnection(): boolean {
  return window.location.protocol !== "https:";
}

/** describedBy は空でない id をつないで aria-describedby の値にする。 */
export function describedBy(
  ...ids: (string | false | null | undefined)[]
): string | undefined {
  const joined = ids.filter((id): id is string => typeof id === "string" && id !== "");
  return joined.length === 0 ? undefined : joined.join(" ");
}

export function CredentialScreen({
  title,
  description,
  onSubmit,
  children,
}: {
  title: string;
  description?: string;
  onSubmit: () => void;
  children: ReactNode;
}) {
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onSubmit();
  };

  return (
    <div className="min-h-dvh bg-bg px-4 pt-16 pb-16 sm:pt-24">
      <main className="mx-auto w-full max-w-sm rounded-lg bg-surface p-6 shadow-card sm:p-8">
        {/* 送信は web/src/api/auth.ts が行う。method="post" を付けない。 */}
        <form noValidate onSubmit={submit} className="flex flex-col gap-5">
          <p className="flex items-center gap-2 text-base font-semibold tracking-tight text-fg select-none">
            <span className="size-2.5 rounded-full bg-accent" aria-hidden="true" />
            vv
          </p>
          <div className="flex flex-col gap-1.5">
            <h1 className="text-xl font-semibold text-fg">{title}</h1>
            {description !== undefined && (
              <p className="text-sm leading-6 text-fg-muted">{description}</p>
            )}
          </div>
          {children}
        </form>
      </main>
    </div>
  );
}

export interface CredentialFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  id: string;
  label: string;
}

export const CredentialField = forwardRef<HTMLInputElement, CredentialFieldProps>(
  function CredentialField({ id, label, className, ...rest }, ref) {
    return (
      <div className="flex flex-col">
        <label htmlFor={id} className="mb-1 text-xs font-medium text-fg-muted">
          {label}
        </label>
        <input
          ref={ref}
          id={id}
          className={cn(
            "h-9 w-full rounded-sm border border-border bg-field px-3 text-sm text-fg focus:border-accent focus:outline-none",
            className,
          )}
          {...rest}
        />
      </div>
    );
  },
);

/** UsernameField は両画面で同じ、ユーザー名の入力である（ui-design.md「Fields」）。 */
export const UsernameField = forwardRef<
  HTMLInputElement,
  Omit<CredentialFieldProps, "label" | "name" | "type" | "autoComplete">
>(function UsernameField(props, ref) {
  return (
    <CredentialField
      ref={ref}
      label="ユーザー名"
      name="username"
      type="text"
      autoComplete="username"
      autoCapitalize="none"
      autoCorrect="off"
      spellCheck={false}
      {...props}
    />
  );
});

/** FailureLine は失敗の行である。出た時点で読まれる。 */
export function FailureLine({ message }: { message: string | null }) {
  if (message === null) return null;
  return (
    <p id={FAILURE_ID} role="alert" className="flex gap-2 text-sm text-danger">
      <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span>{message}</span>
    </p>
  );
}

/**
 * ConnectionWarning は HTTP のときだけ出す接続の警告である。HTTPS ではこの行を出さず、
 * 「安全な接続です」のような肯定の文も出さない（ui-design.md「Connection warning」）。
 */
export function ConnectionWarning() {
  if (!isInsecureConnection()) return null;
  return (
    <p
      id={CONNECTION_WARNING_ID}
      className="flex gap-2 border-l-2 border-warning-strong pl-3 text-sm leading-6 text-warning"
    >
      <ShieldAlert className="mt-1 size-4 shrink-0" aria-hidden="true" />
      <span>
        この接続は暗号化されていません。ユーザー名、パスワード、ログイン状態は通信路で読み取られるおそれがあります。ログインは通信の盗聴を防ぎません
      </span>
    </p>
  );
}

/** connectionWarningId は警告を出すときだけその id を返す。 */
export function connectionWarningId(): string | undefined {
  return isInsecureConnection() ? CONNECTION_WARNING_ID : undefined;
}

import { CircleAlert, ShieldAlert } from "lucide-react";
import type { ComponentProps, FormEvent, ReactNode } from "react";

import { t, type UiText } from "../i18n";
import { CenteredForm } from "../ui/patterns/centered-form";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";

// 初回設定画面とログイン画面が共有する骨格と部品
// （specs/016-single-account-auth/ui-design.md「Credential screens」）。画面は中央フォームの型
// （web/registry/rules/patterns.md「Centered form」）で組み、上の帯にワードマークを置く。

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
  submit,
  footer,
  children,
}: {
  title: UiText;
  description?: UiText;
  onSubmit: () => void;
  /** 送信の Button（default、1 つ）。 */
  submit: ReactNode;
  /** カードの下の補足（ログイン画面へのリンクなど）。 */
  footer?: ReactNode;
  children: ReactNode;
}) {
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onSubmit();
  };

  return (
    <div className="flex min-h-dvh flex-col bg-background">
      <header className="flex h-navbar shrink-0 items-center px-4">
        <img
          src="/brand/vvmdm-wordmark-cyan.svg"
          alt={t.common.appName}
          className="h-9 w-auto max-w-full object-contain object-left"
        />
      </header>
      <main className="flex flex-1 flex-col">
        {/* 送信は web/src/api/auth.ts が行う。method="post" を付けない。 */}
        <CenteredForm
          title={title}
          description={description}
          onSubmit={handleSubmit}
          submit={submit}
          footer={footer}
        >
          <ConnectionWarning />
          {children}
        </CenteredForm>
      </main>
    </div>
  );
}

export interface CredentialFieldProps extends ComponentProps<"input"> {
  id: string;
  label: UiText;
  error?: UiText | null;
}

/** CredentialField はラベル付きの 1 行の入力と、その欄を名指しする失敗の行である。 */
export function CredentialField({ id, label, error, ...rest }: CredentialFieldProps) {
  return (
    <Field data-invalid={rest["aria-invalid"] === true || undefined}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input id={id} {...rest} />
      {error && <FieldError id={FAILURE_ID}>{error}</FieldError>}
    </Field>
  );
}

/** UsernameField は両画面で同じ、ユーザー名の入力である（ui-design.md「Fields」）。 */
export function UsernameField(
  props: Omit<CredentialFieldProps, "label" | "name" | "type" | "autoComplete">,
) {
  return (
    <CredentialField
      label={t.auth.fields.username}
      name="username"
      type="text"
      autoComplete="username"
      autoCapitalize="none"
      autoCorrect="off"
      spellCheck={false}
      {...props}
    />
  );
}

/** FailureLine は欄を名指ししない失敗の行である。出た時点で読まれる。 */
export function FailureLine({ message }: { message: UiText | null }) {
  if (message === null) return null;
  return (
    <Alert id={FAILURE_ID} variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{message}</AlertTitle>
    </Alert>
  );
}

/**
 * ConnectionWarning は HTTP のときだけ出す接続の警告である。HTTPS ではこの行を出さず、
 * 「This connection is secure」のような肯定の文も出さない（ui-design.md「Connection warning」）。
 * 画面を開いた時点からある注意なので、読み上げを割り込ませる alert にはしない。
 */
export function ConnectionWarning() {
  if (!isInsecureConnection()) return null;
  return (
    <Alert id={CONNECTION_WARNING_ID} variant="warning" role="note">
      <ShieldAlert aria-hidden="true" />
      <AlertTitle className="font-normal">{t.auth.connectionWarning}</AlertTitle>
    </Alert>
  );
}

/** connectionWarningId は警告を出すときだけその id を返す。 */
export function connectionWarningId(): string | undefined {
  return isInsecureConnection() ? CONNECTION_WARNING_ID : undefined;
}

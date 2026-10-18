import { CircleAlert, ShieldAlert } from "lucide-react";
import type { ComponentProps, FormEvent, ReactNode } from "react";

import { t, type UiText } from "../i18n";
import { CenteredForm } from "../ui/patterns/centered-form";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";

/** Inputs and the primary action reference this warning via aria-describedby. */
export const CONNECTION_WARNING_ID = "connection-warning";
/** Setup validation references this failure from the affected field. */
export const FAILURE_ID = "credential-failure";

/** Check HTTPS directly: isSecureContext also accepts HTTP loopback addresses. */
export function isInsecureConnection(): boolean {
  return window.location.protocol !== "https:";
}

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
  submit: ReactNode;
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
        {/* api/auth.ts submits the form; do not set method="post". */}
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

export function CredentialField({ id, label, error, ...rest }: CredentialFieldProps) {
  return (
    <Field data-invalid={rest["aria-invalid"] === true || undefined}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input id={id} {...rest} />
      {error && <FieldError id={FAILURE_ID}>{error}</FieldError>}
    </Field>
  );
}

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

export function FailureLine({ message }: { message: UiText | null }) {
  if (message === null) return null;
  return (
    <Alert id={FAILURE_ID} variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{message}</AlertTitle>
    </Alert>
  );
}

/** Show only on HTTP; avoid an alert because this warning is present at page load. */
export function ConnectionWarning() {
  if (!isInsecureConnection()) return null;
  return (
    <Alert id={CONNECTION_WARNING_ID} variant="warning" role="note">
      <ShieldAlert aria-hidden="true" />
      <AlertTitle className="font-normal">{t.auth.connectionWarning}</AlertTitle>
    </Alert>
  );
}

export function connectionWarningId(): string | undefined {
  return isInsecureConnection() ? CONNECTION_WARNING_ID : undefined;
}

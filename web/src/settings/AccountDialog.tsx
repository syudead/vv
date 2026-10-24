import { CircleAlert } from "lucide-react";
import { type FormEvent, useRef, useState } from "react";

import { RequestFailed, updateAccount, type Account } from "../api/client";
import { CredentialField, FAILURE_ID } from "../auth/CredentialScreen";
import { usernameProblem } from "../auth/SetupPage";
import { errorText, t, type UiText } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Spinner } from "../ui/shadcn/spinner";

export type AccountChange = "username" | "password";

type Field = "username" | "newPassword" | "confirm" | "current";

interface Draft {
  username: string;
  newPassword: string;
  confirm: string;
  current: string;
}

interface Invalid {
  field: Field;
  message: UiText;
}

const text = t.settings.account;

/** Client checks are advisory; the server owns the username and password rules. */
export function validateAccountChange(
  change: AccountChange,
  draft: Draft,
  account: Account,
): Invalid | null {
  if (change === "username") {
    const problem = usernameProblem(draft.username);
    if (problem !== null) return { field: "username", message: problem };
    if (draft.username === account.username) {
      return { field: "username", message: text.usernameUnchanged };
    }
  } else if (draft.newPassword === "") {
    return { field: "newPassword", message: t.auth.setup.passwordRequired };
  } else if (draft.confirm !== draft.newPassword) {
    return { field: "confirm", message: t.auth.setup.passwordMismatch };
  }
  if (draft.current === "") {
    return { field: "current", message: text.currentPasswordRequired };
  }
  return null;
}

/** A refusal tied to one field, or null when the failure belongs to the whole dialog. */
function fieldRefusal(change: AccountChange, error: unknown): Invalid | null {
  if (!(error instanceof RequestFailed)) return null;
  if (error.status === 403 && error.code === "invalid_credentials") {
    return { field: "current", message: text.currentPasswordWrong };
  }
  if (error.status === 400) {
    return {
      field: change === "username" ? "username" : "newPassword",
      message: errorText(error),
    };
  }
  return null;
}

/** Mount only while open so the drafted passwords are discarded on close. */
export default function AccountDialog({
  change,
  account,
  onClose,
  onDone,
}: {
  change: AccountChange;
  account: Account;
  onClose: () => void;
  onDone: (account: Account) => void;
}) {
  const [draft, setDraft] = useState<Draft>({
    username: account.username,
    newPassword: "",
    confirm: "",
    current: "",
  });
  const [invalid, setInvalid] = useState<Invalid | null>(null);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);
  const refs = {
    username: useRef<HTMLInputElement>(null),
    newPassword: useRef<HTMLInputElement>(null),
    confirm: useRef<HTMLInputElement>(null),
    current: useRef<HTMLInputElement>(null),
  };

  const refuse = (refusal: Invalid) => {
    setInvalid(refusal);
    setFailure(null);
    refs[refusal.field].current?.focus();
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (pending) return;
    const refusal = validateAccountChange(change, draft, account);
    if (refusal !== null) {
      refuse(refusal);
      return;
    }
    setPending(true);
    setInvalid(null);
    setFailure(null);
    updateAccount({
      currentPassword: draft.current,
      username: change === "username" ? draft.username : account.username,
      ...(change === "password" && { newPassword: draft.newPassword }),
    }).then(
      (saved) => {
        setPending(false);
        onDone(saved);
      },
      (error: unknown) => {
        setPending(false);
        const refused = fieldRefusal(change, error);
        if (refused !== null) refuse(refused);
        else setFailure(text.failed(errorText(error)));
      },
    );
  };

  const field = (name: Field) => ({
    ref: refs[name],
    value: draft[name],
    onChange: (event: { target: { value: string } }) => {
      setDraft((current) => ({ ...current, [name]: event.target.value }));
      if (invalid?.field === name) setInvalid(null);
    },
    error: invalid?.field === name ? invalid.message : null,
    ...(invalid?.field === name && {
      "aria-invalid": true as const,
      "aria-describedby": FAILURE_ID,
    }),
  });

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={change === "username" ? text.usernameTitle : text.passwordTitle}
      description={text.description}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending
            ? text.saving
            : change === "username"
              ? text.changeUsername
              : text.changePassword}
        </>
      }
      pending={pending}
      initialFocus={change === "username" ? refs.username : refs.newPassword}
    >
      {change === "username" ? (
        <CredentialField
          id="account-username"
          label={text.newUsername}
          type="text"
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          {...field("username")}
        />
      ) : (
        <>
          <CredentialField
            id="account-new-password"
            label={text.newPassword}
            type="password"
            autoComplete="new-password"
            {...field("newPassword")}
          />
          <CredentialField
            id="account-confirm-password"
            label={text.confirmPassword}
            type="password"
            autoComplete="new-password"
            {...field("confirm")}
          />
        </>
      )}
      <CredentialField
        id="account-current-password"
        label={text.currentPassword}
        type="password"
        autoComplete="current-password"
        {...field("current")}
      />
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </FormDialog>
  );
}

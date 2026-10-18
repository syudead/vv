import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { setupAccount } from "../api/auth";
import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  connectionWarningId,
  CredentialField,
  CredentialScreen,
  describedBy,
  FAILURE_ID,
  FailureLine,
  UsernameField,
} from "./CredentialScreen";
import { assignPage } from "./pageNavigation";

/** Count Unicode code points, not UTF-16 code units. */
const MAX_USERNAME_LENGTH = 128;

type SetupField = "username" | "password" | "confirm";

/** Client validation is advisory; internal/domain owns the username rules. Check emptiness separately. */
function usernameBreaksRule(username: string): boolean {
  return (
    Array.from(username).length > MAX_USERNAME_LENGTH ||
    /\p{Cc}/u.test(username) ||
    // Match the server's Unicode White_Space property; JavaScript \s also includes U+FEFF.
    /^\p{White_Space}|\p{White_Space}$/u.test(username)
  );
}

interface Invalid {
  field: SetupField;
  message: UiText;
}

export function validateSetup(
  username: string,
  password: string,
  confirm: string,
): Invalid | null {
  if (username === "") {
    return { field: "username", message: t.auth.setup.usernameRequired };
  }
  if (usernameBreaksRule(username)) {
    return {
      field: "username",
      message: t.auth.setup.usernameRule(MAX_USERNAME_LENGTH),
    };
  }
  if (password === "") {
    return { field: "password", message: t.auth.setup.passwordRequired };
  }
  if (confirm !== password) {
    return { field: "confirm", message: t.auth.setup.passwordMismatch };
  }
  return null;
}

export default function SetupPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [invalidField, setInvalidField] = useState<SetupField | null>(null);
  const busy = useRef(false);
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const confirmRef = useRef<HTMLInputElement>(null);
  const refs = { username: usernameRef, password: passwordRef, confirm: confirmRef };
  const loginLink = useRef<HTMLAnchorElement>(null);

  // The stable ref makes this run only on mount.
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
        // Another initial setup won; clear credentials and offer login.
        setFailure(t.errors.code.account_already_configured);
        setUsername("");
        setPassword("");
        setConfirm("");
        setConfigured(true);
      } else if (error instanceof RequestFailed && error.status === 400) {
        setFailure(errorText(error));
      } else if (error instanceof RequestFailed) {
        setFailure(t.auth.setup.failed);
      } else {
        setFailure(errorText(error));
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
      title={t.auth.setup.title}
      description={t.auth.setup.description}
      onSubmit={() => void submit()}
      submit={
        <Button
          type="submit"
          disabled={submitting || configured}
          aria-busy={submitting}
          aria-describedby={warning}
        >
          {submitting && (
            <LoaderCircle
              className="animate-spin motion-reduce:animate-none"
              aria-hidden="true"
            />
          )}
          {submitting ? t.auth.setup.submitting : t.auth.setup.submit}
        </Button>
      }
      footer={
        configured && (
          // Reload the page to replace the gate's cached setupRequired state.
          <Button variant="link" size="sm" asChild>
            <a ref={loginLink} href="/login">
              {t.auth.setup.goToLogin}
            </a>
          </Button>
        )
      }
    >
      <UsernameField
        ref={refs.username}
        id="setup-username"
        value={username}
        onChange={(event) => setUsername(event.target.value)}
        {...fieldProps("username")}
        error={invalidField === "username" ? failure : null}
        aria-describedby={describedBy(invalidField === "username" && FAILURE_ID, warning)}
      />
      <CredentialField
        ref={refs.password}
        id="setup-password"
        label={t.auth.fields.password}
        name="new-password"
        type="password"
        autoComplete="new-password"
        value={password}
        onChange={(event) => setPassword(event.target.value)}
        {...fieldProps("password")}
        error={invalidField === "password" ? failure : null}
      />
      <CredentialField
        ref={refs.confirm}
        id="setup-confirm"
        label={t.auth.fields.confirmPassword}
        name="confirm-password"
        type="password"
        autoComplete="new-password"
        value={confirm}
        onChange={(event) => setConfirm(event.target.value)}
        {...fieldProps("confirm")}
        error={invalidField === "confirm" ? failure : null}
      />
      {invalidField === null && <FailureLine message={failure} />}
    </CredentialScreen>
  );
}

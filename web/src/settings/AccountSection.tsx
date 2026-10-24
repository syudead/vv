import { useCallback, useEffect, useState } from "react";

import { getAccount, type Account } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";
import AccountDialog, { type AccountChange } from "./AccountDialog";

type Load =
  | { kind: "loading" }
  | { kind: "loaded"; account: Account }
  | { kind: "failed"; reason: UiText };

const text = t.settings.account;

function useAccount(): [Load, () => void, (account: Account) => void] {
  const [state, setState] = useState<Load>({ kind: "loading" });
  const load = useCallback((signal?: AbortSignal) => {
    setState({ kind: "loading" });
    getAccount(signal).then(
      (account) => setState({ kind: "loaded", account }),
      (failure: unknown) => {
        if (!signal?.aborted) setState({ kind: "failed", reason: errorText(failure) });
      },
    );
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);
  const set = useCallback(
    (account: Account) => setState({ kind: "loaded", account }),
    [],
  );
  return [state, () => load(), set];
}

/** The username and password change through dialogs that ask for the current password. */
export default function AccountSection() {
  const toast = useToast();
  const [state, reload, setAccount] = useAccount();
  const [open, setOpen] = useState<AccountChange | null>(null);

  if (state.kind === "failed") {
    return (
      <PageSection title={text.heading} description={text.description}>
        <div>
          <ErrorState
            title={text.loadFailed(state.reason)}
            retryLabel={t.common.retry}
            onRetry={reload}
          />
        </div>
      </PageSection>
    );
  }

  const account = state.kind === "loaded" ? state.account : null;
  return (
    <PageSection title={text.heading} description={text.description}>
      <FormRow
        label={text.username}
        description={account?.username}
        htmlFor="account-change-username"
      >
        <Button
          id="account-change-username"
          variant="outline"
          aria-label={text.changeUsername}
          size="sm"
          disabled={account === null}
          onClick={() => setOpen("username")}
        >
          {text.changeUsername}
        </Button>
      </FormRow>
      <FormRow
        label={text.password}
        description={text.passwordDescription}
        htmlFor="account-change-password"
      >
        <Button
          id="account-change-password"
          variant="outline"
          aria-label={text.changePassword}
          size="sm"
          disabled={account === null}
          onClick={() => setOpen("password")}
        >
          {text.changePassword}
        </Button>
      </FormRow>
      {open !== null && account !== null && (
        <AccountDialog
          change={open}
          account={account}
          onClose={() => setOpen(null)}
          onDone={(saved) => {
            setAccount(saved);
            setOpen(null);
            toast(open === "username" ? text.usernameChanged : text.passwordChanged);
          }}
        />
      )}
    </PageSection>
  );
}

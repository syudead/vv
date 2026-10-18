import { TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  getNetworkSettings,
  getPluginProxySettings,
  RequestFailed,
  updateNetworkSettings,
  type NetworkSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Switch } from "../ui/shadcn/switch";

import { PluginProxyRow } from "./PluginProxyRow";
import { SaveStatus } from "./SaveStatus";
import { useSwitchSave } from "./useSwitchSave";

/** absent means the desktop-only endpoint returned 404. */
type Load<T> =
  | { kind: "loading" }
  | { kind: "loaded"; value: T }
  | { kind: "absent" }
  | { kind: "failed"; reason: UiText };

const LOADING = { kind: "loading" } as const;

function useLoad<T>(
  get: (signal?: AbortSignal) => Promise<T>,
  absentOn404: boolean,
): [Load<T>, (signal?: AbortSignal) => Promise<void>, (value: T) => void] {
  const [state, setState] = useState<Load<T>>(LOADING);
  const load = useCallback(
    async (signal?: AbortSignal) => {
      setState(LOADING);
      try {
        const value = await get(signal);
        setState({ kind: "loaded", value });
      } catch (failure) {
        if (signal?.aborted) return;
        if (absentOn404 && failure instanceof RequestFailed && failure.status === 404) {
          setState({ kind: "absent" });
          return;
        }
        setState({ kind: "failed", reason: errorText(failure) });
      }
    },
    [get, absentOn404],
  );
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);
  const set = useCallback((value: T) => setState({ kind: "loaded", value }), []);
  return [state, load, set];
}

/**
 * Network 404 means a non-desktop build; proxy settings remain available. Show independent load
 * failures per row and rollback the optimistic LAN toggle on save failure.
 */
export default function NetworkSection() {
  const text = t.settings.network;
  const [network, loadNetwork, setNetwork] = useLoad(getNetworkSettings, true);
  const [proxy, loadProxy] = useLoad(getPluginProxySettings, false);
  const settings = network.kind === "loaded" ? network.value : null;

  const { saving, saveError, toggle } = useSwitchSave<NetworkSettings>({
    settings,
    current: (saved) => saved.lanAccess,
    save: updateNetworkSettings,
    saved: setNetwork,
    saveFailed: text.saveFailed,
  });

  const anyLoaded = network.kind === "loaded" || proxy.kind === "loaded";
  const anyLoading = network.kind === "loading" || proxy.kind === "loading";
  const failure = firstFailure(network, proxy);
  if (!anyLoaded && (anyLoading || failure === null)) return null;

  const errorState = (reason: UiText, retry: () => void) => (
    <div>
      <ErrorState
        title={text.loadFailed(reason)}
        retryLabel={t.common.retry}
        onRetry={retry}
      />
    </div>
  );

  if (!anyLoaded && failure !== null) {
    return (
      <PageSection title={text.heading} description={text.description}>
        {errorState(failure, () => {
          if (network.kind === "failed") void loadNetwork();
          void loadProxy();
        })}
      </PageSection>
    );
  }

  return (
    <PageSection title={text.heading} description={text.description}>
      {network.kind === "failed" && errorState(network.reason, () => void loadNetwork())}
      {network.kind === "absent" && (
        <p className="text-sm text-muted-foreground">{text.notDesktop}</p>
      )}
      {settings !== null && (
        <LANAccess
          settings={settings}
          checked={saving ?? settings.lanAccess}
          saving={saving !== null}
          saveError={saveError}
          toggle={toggle}
        />
      )}
      {proxy.kind === "loaded" && <PluginProxyRow initialURL={proxy.value.url} />}
      {proxy.kind === "failed" && errorState(proxy.reason, () => void loadProxy())}
    </PageSection>
  );
}

function firstFailure(...loads: Load<unknown>[]): UiText | null {
  for (const load of loads) {
    if (load.kind === "failed") return load.reason;
  }
  return null;
}

function LANAccess({
  settings,
  checked,
  saving,
  saveError,
  toggle,
}: {
  settings: NetworkSettings;
  checked: boolean;
  saving: boolean;
  saveError: UiText | null;
  toggle: () => Promise<void>;
}) {
  const text = t.settings.network;
  return (
    <>
      <FormRow
        label={text.lanAccess}
        description={text.lanAccessDescription}
        htmlFor="network-lan-access"
      >
        <Switch
          id="network-lan-access"
          checked={checked}
          aria-disabled={saving || undefined}
          onCheckedChange={() => void toggle()}
        />
      </FormRow>
      <SaveStatus saving={saving} savingText={text.saving} error={saveError} />
      {settings.lanAccess ? (
        settings.addresses.length > 0 ? (
          <div className="flex flex-col gap-2">
            <p className="text-muted-foreground">{text.addresses}</p>
            <ul aria-label={text.addresses} className="flex flex-col gap-1">
              {settings.addresses.map((address) => (
                <li key={address}>
                  <code className="rounded-sm bg-muted px-2 py-1 break-all select-all">
                    {address}
                  </code>
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <p className="text-warning">{text.noAddresses}</p>
        )
      ) : (
        <div>
          <Alert variant="warning" role="note">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{text.cautionHeading}</AlertTitle>
            <AlertDescription>
              <ul className="list-disc pl-4">
                <li>{text.caution.lan}</li>
                <li>{text.caution.firewall}</li>
                <li>{text.caution.http}</li>
              </ul>
            </AlertDescription>
          </Alert>
        </div>
      )}
    </>
  );
}

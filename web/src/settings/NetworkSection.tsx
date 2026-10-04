import { CircleAlert, LoaderCircle, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  getNetworkSettings,
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

/**
 * NetworkSection は設定画面の「Network」区画である。Windows デスクトップ版で、所有者が
 * LAN からの接続を許可し、許可中に開くアドレスを見る（specs/037-windows-app/research.md R-15）。
 *
 * `GET /api/settings/network` が `404` のとき（デスクトップ版でない）は何も出さない。読み終える
 * までも出さない（デスクトップ版でない画面に区画が一瞬出ないように）。
 *
 * スイッチを押すと、送信中は押した先の状態を示し、応答で表示を置き換える。失敗
 * （`409` `listen_failed` など）では誤りを出し、スイッチを元の状態に戻す
 * （contracts/network-settings-api.md §3）。
 */
export default function NetworkSection() {
  const text = t.settings.network;
  const [settings, setSettings] = useState<NetworkSettings | null>(null);
  const [notDesktop, setNotDesktop] = useState(false);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState<boolean | null>(null);
  const [saveError, setSaveError] = useState<UiText | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoadError(null);
    try {
      setSettings(await getNetworkSettings(signal));
    } catch (failure) {
      if (signal?.aborted) return;
      if (failure instanceof RequestFailed && failure.status === 404) {
        setNotDesktop(true);
        return;
      }
      setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  if (notDesktop || (settings === null && loadError === null)) return null;

  const toggle = async () => {
    if (settings === null || saving !== null) return;
    const next = !settings.lanAccess;
    setSaveError(null);
    setSaving(next);
    try {
      setSettings(await updateNetworkSettings(next));
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
    } finally {
      setSaving(null);
    }
  };

  const checked = saving ?? settings?.lanAccess ?? false;

  return (
    <PageSection title={text.heading} description={text.description}>
      {loadError !== null && settings === null && (
        <div>
          <ErrorState
            title={text.loadFailed(loadError)}
            retryLabel={t.common.retry}
            onRetry={() => void load()}
          />
        </div>
      )}
      {settings !== null && (
        <FormRow label={text.lanAccess} htmlFor="network-lan-access">
          <Switch
            id="network-lan-access"
            checked={checked}
            aria-disabled={saving !== null || undefined}
            onCheckedChange={() => void toggle()}
          />
        </FormRow>
      )}
      {saving !== null && (
        <p
          role="status"
          className="flex items-center gap-2 text-xs text-muted-foreground"
        >
          <LoaderCircle
            aria-hidden="true"
            className="size-4 animate-spin motion-reduce:animate-none"
          />
          {text.saving}
        </p>
      )}
      {saveError !== null && (
        <div>
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{saveError}</AlertTitle>
          </Alert>
        </div>
      )}
      {settings !== null &&
        (settings.lanAccess ? (
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
        ))}
    </PageSection>
  );
}

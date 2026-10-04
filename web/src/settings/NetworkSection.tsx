import { AlertTriangle, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  getNetworkSettings,
  RequestFailed,
  updateNetworkSettings,
  type NetworkSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import Button from "../ui/legacy/Button";

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
    <section
      aria-labelledby="network-heading"
      className="mt-8 rounded-lg border border-border bg-surface p-4 sm:p-5"
    >
      <div className="border-b border-border pb-4">
        <h2 id="network-heading" className="text-base font-semibold">
          {text.heading}
        </h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">
          {text.description}
        </p>
      </div>

      {loadError !== null && settings === null && (
        <div className="flex flex-col items-start gap-3 py-6">
          <p role="alert" className="text-sm text-danger">
            {text.loadFailed(loadError)}
          </p>
          <Button onClick={() => void load()}>{t.common.retry}</Button>
        </div>
      )}
      {settings !== null && (
        <div className="pt-4">
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              role="switch"
              id="network-lan-access"
              aria-checked={checked}
              aria-labelledby="network-lan-access-label"
              aria-disabled={saving !== null || undefined}
              onClick={() => void toggle()}
              className={cn(
                "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border border-control-border transition-colors duration-150 aria-disabled:cursor-default aria-disabled:opacity-60",
                checked ? "bg-primary" : "bg-field",
              )}
            >
              <span
                aria-hidden="true"
                className={cn(
                  "inline-block size-4 rounded-full transition-transform duration-150 motion-reduce:transition-none",
                  checked
                    ? "translate-x-6 bg-primary-foreground"
                    : "translate-x-1 bg-fg-muted",
                )}
              />
            </button>
            <label
              id="network-lan-access-label"
              htmlFor="network-lan-access"
              className="cursor-pointer text-fg"
            >
              {text.lanAccess}
            </label>
            {saving !== null && (
              <span
                role="status"
                className="flex items-center gap-1.5 text-xs text-fg-muted"
              >
                <LoaderCircle
                  aria-hidden="true"
                  className="size-3.5 animate-spin motion-reduce:animate-none"
                />
                {text.saving}
              </span>
            )}
          </div>
          {saveError !== null && (
            <p role="alert" className="mt-3 text-sm text-danger">
              {saveError}
            </p>
          )}

          {settings.lanAccess ? (
            <div className="mt-4">
              {settings.addresses.length > 0 ? (
                <>
                  <p className="text-sm text-fg-muted">{text.addresses}</p>
                  <ul aria-label={text.addresses} className="mt-2 space-y-1">
                    {settings.addresses.map((address) => (
                      <li key={address}>
                        <code className="rounded-md border border-control-border bg-field px-2 py-1 text-sm break-all select-all">
                          {address}
                        </code>
                      </li>
                    ))}
                  </ul>
                </>
              ) : (
                <p className="text-sm text-warning">{text.noAddresses}</p>
              )}
            </div>
          ) : (
            <div className="mt-4 rounded-md border border-border p-3">
              <p className="flex items-center gap-2 text-sm font-medium text-warning">
                <AlertTriangle aria-hidden="true" className="size-4 shrink-0" />
                {text.cautionHeading}
              </p>
              <ul className="mt-2 list-disc space-y-1 pl-6 text-sm leading-6 text-fg-muted">
                <li>{text.caution.lan}</li>
                <li>{text.caution.firewall}</li>
                <li>{text.caution.http}</li>
              </ul>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

import { CircleAlert, LoaderCircle, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  type AutoImportSettings,
  getAutoImportSettings,
  isAborted,
  updateAutoImportSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { inProgress, useScan } from "../shell/ScanProvider";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Switch } from "../ui/shadcn/switch";

/** pollMs は監視を張っている最中（starting）に状態を読み直す間隔である。 */
const pollMs = 2000;

function stateLine(settings: AutoImportSettings): UiText {
  const text = t.settings.autoImport.state;
  if (!settings.enabled) return text.off;
  switch (settings.watch.state) {
    case "off":
      return text.noFolder;
    case "starting":
      return text.starting;
    case "active":
      return text.active;
    case "limited":
      return text.limited;
  }
}

function ProblemAlert({ watch }: { watch: AutoImportSettings["watch"] }) {
  if (watch.state !== "limited" || watch.problem === undefined) return null;
  const text = t.settings.autoImport;
  const problem = text.problem[watch.problem];
  // 道筋を持たない問題（events_lost）は、文がそれだけで完結している。
  const path = watch.problem === "events_lost" ? null : (watch.path ?? text.unknownPath);
  return (
    <div>
      <Alert variant="warning" role="note">
        <TriangleAlert aria-hidden="true" />
        <AlertTitle>{problem.title}</AlertTitle>
        <AlertDescription>
          <p>
            {problem.before}
            {path !== null &&
              (watch.path !== undefined ? (
                <code className="break-all">{path}</code>
              ) : (
                path
              ))}
            {problem.after}
          </p>
        </AlertDescription>
      </Alert>
    </div>
  );
}

/**
 * AutoImportSection は設定の「Auto-import」区画である。メディアフォルダの変化を拾って
 * 変わったところだけ取り込む機能を、所有者が切り替える（specs/042-folder-watch-import/ui-design.md
 * 「Settings: Auto-import section」）。
 *
 * 状態は区画が開いたとき、ウィンドウへ戻ったとき、メディアフォルダを変えたあと
 * （`reloadToken`）、直近の取り込みが終わったときに読み直し、監視を張っている最中
 * （`starting`）は 2 秒ごとに読み直す。オンにしても、バーも通知も出さない。
 */
export default function AutoImportSection({ reloadToken }: { reloadToken: number }) {
  const text = t.settings.autoImport;
  const scan = useScan();
  const [settings, setSettings] = useState<AutoImportSettings | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState<boolean | null>(null);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  // 読み込みと保存は重なる。いちばん新しく始めたものの応答だけを使う。
  const revision = useRef(0);
  const controller = useRef<AbortController | null>(null);
  // 保存（PUT）の最中は読み直さない。重なった読み込みの古い値が、保存の結果を上書きしたり
  // 保存の応答を捨てさせたりしないよう、保存が終わるまで読み込みを止める。
  const saveInFlight = useRef(false);
  const settingsRef = useRef(settings);
  settingsRef.current = settings;

  const load = useCallback(async () => {
    if (saveInFlight.current) return;
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    revision.current += 1;
    const mine = revision.current;
    try {
      const next = await getAutoImportSettings(abort.signal);
      if (mine !== revision.current) return;
      setSettings(next);
      setLoadError(null);
    } catch (failure) {
      if (isAborted(failure) || mine !== revision.current) return;
      // 読めている状態は残す。読み直しの失敗で、区画を壊さない。
      if (settingsRef.current === null) setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, reloadToken]);

  useEffect(() => {
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [load]);

  useEffect(
    () => () => {
      controller.current?.abort();
    },
    [],
  );

  const starting = settings?.watch.state === "starting";
  useEffect(() => {
    if (!starting) return;
    const timer = window.setInterval(() => void load(), pollMs);
    return () => window.clearInterval(timer);
  }, [load, starting]);

  // 直近の取り込みが終わったら読み直す。問題は、手動の取り込みの完了で消えることがある。
  // 最初に見た値は、開いたときの読み込みが担う。
  const endedId = scan.scan !== null && !inProgress(scan.scan) ? scan.scan.id : null;
  const seenEnded = useRef<number | null | undefined>(undefined);
  useEffect(() => {
    if (!scan.loaded) return;
    if (seenEnded.current === undefined) {
      seenEnded.current = endedId;
      return;
    }
    if (endedId !== null && endedId !== seenEnded.current) void load();
    seenEnded.current = endedId;
  }, [endedId, load, scan.loaded]);

  const toggle = async () => {
    if (settings === null || saving !== null) return;
    const next = !settings.enabled;
    // 保存より前に始めた読み込みの応答は使わない。
    controller.current?.abort();
    revision.current += 1;
    saveInFlight.current = true;
    setSaveError(null);
    setSaving(next);
    try {
      setSettings(await updateAutoImportSettings(next));
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
    } finally {
      saveInFlight.current = false;
      setSaving(null);
    }
  };

  const checked = saving ?? settings?.enabled ?? false;

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
      {loadError === null && settings === null && (
        <FormRow label={text.label} htmlFor="auto-import-enabled">
          <Skeleton
            role="status"
            aria-label={text.loading}
            className="h-5 w-9 rounded-full"
          />
        </FormRow>
      )}
      {settings !== null && (
        <FormRow
          label={text.label}
          description={stateLine(settings)}
          htmlFor="auto-import-enabled"
        >
          <Switch
            id="auto-import-enabled"
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
      {settings !== null && <ProblemAlert watch={settings.watch} />}
    </PageSection>
  );
}

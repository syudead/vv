import { CircleAlert, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  getVersionJudgingSettings,
  isAborted,
  updateVersionJudgingSettings,
  type VersionJudgingSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Switch } from "../ui/shadcn/switch";

/**
 * VersionJudgingSection は設定の「Duplicate judging」区画である。「同じ動画かもしれない」候補の
 * 自動の判定を、所有者が 1 つのスイッチで入れたり切ったりする
 * （specs/054-auto-dup/ui-design.md「Settings: Duplicate judging section」）。
 *
 * スイッチを押すとすぐに保存し、保存ボタンも成功の通知も無い。Ollama の接続先は読まず、
 * 判定の進み具合も出さない（それは Duplicates の画面が見せる）。
 */
export default function VersionJudgingSection() {
  const text = t.settings.versionJudging;
  const [settings, setSettings] = useState<VersionJudgingSettings | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState<boolean | null>(null);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  const controller = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    setLoadError(null);
    try {
      setSettings(await getVersionJudgingSettings(abort.signal));
    } catch (failure) {
      if (isAborted(failure)) return;
      setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
    return () => controller.current?.abort();
  }, [load]);

  const toggle = async () => {
    if (settings === null || saving !== null) return;
    const next = !settings.enabled;
    setSaveError(null);
    setSaving(next);
    try {
      setSettings(await updateVersionJudgingSettings(next));
    } catch (failure) {
      // スイッチは保存した値に戻る（saving を解くと settings.enabled を映す）。
      setSaveError(text.saveFailed(errorText(failure)));
    } finally {
      setSaving(null);
    }
  };

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
      {loadError === null && (
        <FormRow
          label={text.label}
          description={text.hint}
          htmlFor="version-judging-enabled"
        >
          {settings === null ? (
            // Skeleton は既定で読み上げから隠れる。読み込み中であることは読み上げに出す。
            <Skeleton
              aria-hidden={false}
              role="status"
              aria-label={text.loading}
              className="h-5 w-9 rounded-full"
            />
          ) : (
            <Switch
              id="version-judging-enabled"
              checked={saving ?? settings.enabled}
              aria-disabled={saving !== null || undefined}
              onCheckedChange={() => void toggle()}
            />
          )}
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
    </PageSection>
  );
}

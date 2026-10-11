import { CircleAlert, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  type AutoTaggingScope,
  type AutoTaggingSettings,
  getAutoTaggingSettings,
  isAborted,
  startAutoTagging,
  updateAutoTaggingSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Switch } from "../ui/shadcn/switch";
import { useToast } from "../ui/Toast";

/** pollMs は判定が待ち行列に残っている間に件数を読み直す間隔である。 */
const pollMs = 3000;

/** Draft は画面で編集中の値である。閾値は入力の文字列のまま持つ。 */
interface Draft {
  enabled: boolean;
  afterImport: boolean;
  threshold: string;
  tentativeThreshold: string;
}

function toDraft(settings: AutoTaggingSettings): Draft {
  return {
    enabled: settings.enabled,
    afterImport: settings.afterImport,
    threshold: String(settings.threshold),
    tentativeThreshold: String(settings.tentativeThreshold),
  };
}

/** parseThreshold は入力を 0 より大きく 1 以下の数として読む。読めなければ null。 */
export function parseThreshold(value: string): number | null {
  if (value.trim() === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 && parsed <= 1 ? parsed : null;
}

/**
 * AutoTaggingSection は設定の「Auto-tagging」区画である。機能の入切、取り込みのあとの
 * 自動の判定と 2 つの確率を設定し、機能が入ならライブラリの動画を判定に回す
 * （docs/design-docs/auto-tagging.md）。Ollama の接続先は OllamaSection が持つ。判定が
 * 待ち行列に残っている間は件数を読み直す。
 */
export default function AutoTaggingSection() {
  const text = t.settings.autoTagging;
  const toast = useToast();
  const [settings, setSettings] = useState<AutoTaggingSettings | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  const [starting, setStarting] = useState<AutoTaggingScope | null>(null);
  const controller = useRef<AbortController | null>(null);
  const settingsRef = useRef(settings);
  settingsRef.current = settings;

  const load = useCallback(async () => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    try {
      const next = await getAutoTaggingSettings(abort.signal);
      if (abort.signal.aborted) return;
      setSettings(next);
      // 編集中の値は読み直しで消さない。初めて読めたときだけ下書きを作る。
      setDraft((current) => current ?? toDraft(next));
      setLoadError(null);
    } catch (failure) {
      if (isAborted(failure)) return;
      if (settingsRef.current === null) setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
    return () => controller.current?.abort();
  }, [load]);

  const active = settings !== null && settings.queue.queued + settings.queue.running > 0;
  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => void load(), pollMs);
    return () => window.clearInterval(timer);
  }, [active, load]);

  const threshold = draft === null ? null : parseThreshold(draft.threshold);
  const tentativeThreshold =
    draft === null ? null : parseThreshold(draft.tentativeThreshold);
  // 仮に付ける確率は、確定して付ける確率以下でなければならない。
  const tentativeValid =
    tentativeThreshold !== null &&
    (threshold === null || tentativeThreshold <= threshold);
  const valid = threshold !== null && tentativeThreshold !== null && tentativeValid;

  /** persist は下書きを保存する。保存できたら true を返す。 */
  const persist = async (): Promise<boolean> => {
    if (draft === null || threshold === null || tentativeThreshold === null || !valid)
      return false;
    setSaveError(null);
    try {
      const saved = await updateAutoTaggingSettings({
        enabled: draft.enabled,
        afterImport: draft.afterImport,
        threshold,
        tentativeThreshold,
      });
      setSettings(saved);
      setDraft(toDraft(saved));
      return true;
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
      return false;
    }
  };

  const save = async () => {
    if (draft === null || !valid || saving) return;
    setSaving(true);
    try {
      if (await persist()) toast(text.saved);
    } finally {
      setSaving(false);
    }
  };

  const start = async (scope: AutoTaggingScope) => {
    if (starting !== null) return;
    setStarting(scope);
    try {
      const queued = await startAutoTagging(scope);
      toast(text.started(queued));
      await load();
    } catch (failure) {
      toast(text.startFailed(errorText(failure)));
    } finally {
      setStarting(null);
    }
  };

  const update = (change: Partial<Draft>) => {
    setDraft((current) => (current === null ? current : { ...current, ...change }));
    setSaveError(null);
  };

  if (draft === null || settings === null) {
    return (
      <PageSection title={text.heading} description={text.description}>
        {loadError !== null ? (
          <div>
            <ErrorState
              title={text.loadFailed(loadError)}
              retryLabel={t.common.retry}
              onRetry={() => void load()}
            />
          </div>
        ) : (
          <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
            <Skeleton className="h-9" />
            <Skeleton className="h-9" />
          </div>
        )}
      </PageSection>
    );
  }

  const queue = settings.queue;
  return (
    <PageSection title={text.heading} description={text.description}>
      <FormRow
        label={text.enabled}
        description={text.enabledHint}
        htmlFor="auto-tagging-enabled"
      >
        <Switch
          id="auto-tagging-enabled"
          checked={draft.enabled}
          onCheckedChange={(checked) => update({ enabled: checked })}
        />
      </FormRow>
      <FormRow
        label={text.afterImport}
        description={text.afterImportHint}
        htmlFor="auto-tagging-after-import"
      >
        <Switch
          id="auto-tagging-after-import"
          checked={draft.afterImport}
          onCheckedChange={(checked) => update({ afterImport: checked })}
        />
      </FormRow>
      <FormRow
        label={text.threshold}
        description={text.thresholdHint}
        htmlFor="auto-tagging-threshold"
      >
        <Input
          id="auto-tagging-threshold"
          type="number"
          inputMode="decimal"
          min={0.01}
          max={1}
          step={0.05}
          className="sm:w-16"
          aria-invalid={threshold === null || undefined}
          value={draft.threshold}
          onChange={(event) => update({ threshold: event.target.value })}
        />
      </FormRow>
      <FormRow
        label={text.tentativeThreshold}
        description={text.tentativeThresholdHint}
        htmlFor="auto-tagging-tentative-threshold"
      >
        <Input
          id="auto-tagging-tentative-threshold"
          type="number"
          inputMode="decimal"
          min={0.01}
          max={1}
          step={0.05}
          className="sm:w-16"
          aria-invalid={!tentativeValid || undefined}
          value={draft.tentativeThreshold}
          onChange={(event) => update({ tentativeThreshold: event.target.value })}
        />
      </FormRow>
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            onClick={() => void save()}
            aria-disabled={saving || !valid || undefined}
          >
            {saving && (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            )}
            {saving ? text.saving : text.save}
          </Button>
        </div>
        {saveError !== null && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{saveError}</AlertTitle>
          </Alert>
        )}
      </div>
      {settings.enabled && (
        <div className="flex flex-col gap-2">
          <p role="status" className="text-sm text-muted-foreground">
            {text.queue(queue.queued, queue.running, queue.done, queue.failed)}
          </p>
          {queue.lastError !== undefined && (
            <Alert variant="warning" role="note">
              <CircleAlert aria-hidden="true" />
              <AlertTitle>{text.lastError}</AlertTitle>
              <AlertDescription>
                <p className="break-all">{queue.lastError}</p>
              </AlertDescription>
            </Alert>
          )}
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => void start("missing")}
              aria-disabled={starting !== null || undefined}
            >
              {text.runMissing}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => void start("all")}
              aria-disabled={starting !== null || undefined}
            >
              {text.runAll}
            </Button>
          </div>
        </div>
      )}
    </PageSection>
  );
}

import { CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  type AutoTaggingScope,
  type AutoTaggingSettings,
  checkAutoTagging,
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
  endpoint: string;
  model: string;
  threshold: string;
}

function toDraft(settings: AutoTaggingSettings): Draft {
  return {
    enabled: settings.enabled,
    endpoint: settings.endpoint,
    model: settings.model,
    threshold: String(settings.threshold),
  };
}

/** parseThreshold は入力を 0 より大きく 1 以下の数として読む。読めなければ null。 */
export function parseThreshold(value: string): number | null {
  if (value.trim() === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 && parsed <= 1 ? parsed : null;
}

type CheckState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "ok" }
  | { kind: "failed"; message: UiText };

/**
 * AutoTaggingSection は設定の「Auto-tagging」区画である。Ollama の判定モデル（Clef）の
 * 問い合わせ先・模型・閾値と、取り込みのあとの自動の判定を設定し、ライブラリの動画を
 * 判定に回す（docs/design-docs/auto-tagging.md）。判定が待ち行列に残っている間は件数を
 * 読み直す。
 */
export default function AutoTaggingSection() {
  const text = t.settings.autoTagging;
  const toast = useToast();
  const [settings, setSettings] = useState<AutoTaggingSettings | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  const [check, setCheck] = useState<CheckState>({ kind: "idle" });
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

  const save = async () => {
    if (draft === null || threshold === null || saving) return;
    setSaving(true);
    setSaveError(null);
    try {
      const saved = await updateAutoTaggingSettings({
        enabled: draft.enabled,
        endpoint: draft.endpoint,
        model: draft.model,
        threshold,
      });
      setSettings(saved);
      setDraft(toDraft(saved));
      toast(text.saved);
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
    } finally {
      setSaving(false);
    }
  };

  const runCheck = async () => {
    if (draft === null || check.kind === "checking") return;
    setCheck({ kind: "checking" });
    try {
      const result = await checkAutoTagging(draft.endpoint, draft.model);
      setCheck(
        result.available
          ? { kind: "ok" }
          : { kind: "failed", message: text.checkFailed(result.message ?? "") },
      );
    } catch (failure) {
      setCheck({ kind: "failed", message: text.checkFailed(errorText(failure)) });
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
        label={text.endpoint}
        description={text.endpointHint}
        htmlFor="auto-tagging-endpoint"
      >
        <Input
          id="auto-tagging-endpoint"
          type="url"
          className="sm:w-sm"
          value={draft.endpoint}
          onChange={(event) => update({ endpoint: event.target.value })}
        />
      </FormRow>
      <FormRow
        label={text.model}
        description={text.modelHint}
        htmlFor="auto-tagging-model"
      >
        <Input
          id="auto-tagging-model"
          className="sm:w-sm"
          value={draft.model}
          onChange={(event) => update({ model: event.target.value })}
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
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            onClick={() => void save()}
            aria-disabled={saving || threshold === null || undefined}
          >
            {saving && (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            )}
            {saving ? text.saving : text.save}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => void runCheck()}
            aria-disabled={check.kind === "checking" || undefined}
          >
            {text.check}
          </Button>
        </div>
        {check.kind === "checking" && (
          <p
            role="status"
            className="flex items-center gap-2 text-xs text-muted-foreground"
          >
            <LoaderCircle
              aria-hidden="true"
              className="size-4 animate-spin motion-reduce:animate-none"
            />
            {text.checking}
          </p>
        )}
        {check.kind === "ok" && (
          <p
            role="status"
            className="flex items-center gap-2 text-xs text-muted-foreground"
          >
            <CircleCheck aria-hidden="true" className="size-4" />
            {text.checkOk}
          </p>
        )}
        {check.kind === "failed" && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{check.message}</AlertTitle>
          </Alert>
        )}
        {saveError !== null && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{saveError}</AlertTitle>
          </Alert>
        )}
      </div>
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
    </PageSection>
  );
}

import { CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import {
  type AutoTaggingScope,
  type AutoTaggingSettings,
  checkAutoTaggingClassifier,
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

import { PageLinkText } from "./PageLink";

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

type CheckState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "ok" }
  | { kind: "failed"; message: UiText };

/**
 * ProgressSection は「Auto-tagging」のページの「Progress」節である。見出しの説明は待ち行列の
 * 件数で、中身は判定に回すボタンと直近の失敗である。保存した設定で機能が切なら、切である旨と
 * 入れる場所を出し、ボタンは出さない。
 */
function ProgressSection({
  settings,
  starting,
  locked,
  onStart,
}: {
  settings: AutoTaggingSettings;
  starting: AutoTaggingScope | null;
  locked?: boolean;
  onStart: (scope: AutoTaggingScope) => void;
}) {
  const text = t.settings.autoTagging;
  const queue = settings.queue;
  if (!settings.enabled)
    return (
      <PageSection title={text.progressHeading} description={text.off}>
        <p>{text.offHint}</p>
      </PageSection>
    );
  return (
    <PageSection
      title={text.progressHeading}
      description={
        <span role="status">
          {text.queue(queue.queued, queue.running, queue.done, queue.failed)}
        </span>
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={() => onStart("missing")}
          aria-disabled={locked || starting !== null || undefined}
        >
          {text.runMissing}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => onStart("all")}
          aria-disabled={locked || starting !== null || undefined}
        >
          {text.runAll}
        </Button>
      </div>
      {queue.lastError !== undefined && (
        <div>
          <Alert variant="warning" role="note">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{text.lastError}</AlertTitle>
            <AlertDescription>
              <p className="break-all">{queue.lastError}</p>
            </AlertDescription>
          </Alert>
        </div>
      )}
    </PageSection>
  );
}

/**
 * SectionsLoading は 2 つの節を読んでいる間の形である。読めなければ「Classifier」の節に
 * 読めなかった旨と `Retry` を出す。
 */
function SectionsLoading({
  description,
  loadError,
  onRetry,
}: {
  description: ReactNode;
  loadError: UiText | null;
  onRetry: () => void;
}) {
  const text = t.settings.autoTagging;
  return (
    <>
      {loadError === null && (
        <PageSection title={text.progressHeading}>
          <Skeleton className="h-8" />
        </PageSection>
      )}
      <PageSection title={text.heading} description={description}>
        {loadError !== null ? (
          <div>
            <ErrorState
              title={text.loadFailed(loadError)}
              retryLabel={t.common.retry}
              onRetry={onRetry}
            />
          </div>
        ) : (
          <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
            <Skeleton className="h-9" />
            <Skeleton className="h-9" />
          </div>
        )}
      </PageSection>
    </>
  );
}

/** ClassifierFeedback は「Test judging」の結果と保存の失敗の行である。 */
function ClassifierFeedback({
  check,
  saveError,
}: {
  check: CheckState;
  saveError: UiText | null;
}) {
  const text = t.settings.autoTagging;
  return (
    <>
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
    </>
  );
}

/**
 * AutoTaggingSection は「Auto-tagging」のページの「Progress」と「Classifier」の 2 つの節である。
 * 「Classifier」で機能の入切、取り込みのあとの自動の判定と 2 つの確率を設定し、機能が入なら
 * 「Progress」でライブラリの動画を判定に回す
 * （docs/design-docs/auto-tagging.md）。Ollama の接続先は OllamaSection が持つ。判定が
 * 待ち行列に残っている間は件数を読み直す。「Test judging」は判定の口に 1 問を判定させ、
 * 機能の入切に関係なく使えるかを確かめる。
 *
 * `locked` は Ollama の接続先が保存されていないことである。そのあいだ切り替え、入力、ボタンは値を見せたまま `aria-disabled` にし、`aria-disabled` は
 * それだけでは何も止めないので、各ハンドラは下書きを変えず要求も送らずに返す。`pending` は
 * 接続先をまだ読んでいることで、保存したかが分かるまでは操作を描かず読み込み中の形を出す（R-5 の
 * Loading）。
 */
export default function AutoTaggingSection({
  locked,
  pending,
}: {
  locked?: boolean;
  pending?: boolean;
}) {
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
    if (locked || draft === null || !valid || saving) return;
    setSaving(true);
    try {
      if (await persist()) toast(text.saved);
    } finally {
      setSaving(false);
    }
  };

  const runCheck = async () => {
    if (locked || check.kind === "checking") return;
    setCheck({ kind: "checking" });
    try {
      const result = await checkAutoTaggingClassifier();
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
    if (locked || starting !== null) return;
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
    if (locked) return;
    setDraft((current) => (current === null ? current : { ...current, ...change }));
    setSaveError(null);
  };

  const description = <PageLinkText sentence={text.description} page="connections" />;

  if (pending || draft === null || settings === null)
    return (
      <SectionsLoading
        description={description}
        loadError={loadError}
        onRetry={() => void load()}
      />
    );

  return (
    <>
      <ProgressSection
        settings={settings}
        starting={starting}
        locked={locked}
        onStart={(scope) => void start(scope)}
      />
      <PageSection title={text.heading} description={description}>
        <FormRow
          label={text.enabled}
          description={text.enabledHint}
          htmlFor="auto-tagging-enabled"
        >
          <Switch
            id="auto-tagging-enabled"
            checked={draft.enabled}
            aria-disabled={locked || undefined}
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
            aria-disabled={locked || undefined}
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
            aria-disabled={locked || undefined}
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
            aria-disabled={locked || undefined}
            value={draft.tentativeThreshold}
            onChange={(event) => update({ tentativeThreshold: event.target.value })}
          />
        </FormRow>
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              onClick={() => void save()}
              aria-disabled={locked || saving || !valid || undefined}
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
              aria-disabled={locked || check.kind === "checking" || undefined}
            >
              {text.check}
            </Button>
          </div>
          <ClassifierFeedback check={check} saveError={saveError} />
        </div>
      </PageSection>
    </>
  );
}

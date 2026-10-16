import { CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  checkOllama,
  getOllamaSettings,
  isAborted,
  type OllamaSettings,
  updateOllamaSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Skeleton } from "../ui/shadcn/skeleton";
import { useToast } from "../ui/Toast";

type CheckState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "ok" }
  | { kind: "failed"; message: UiText };

/**
 * OllamaSection は設定の「Ollama」区画である。判定モデル（Clef）を動かす Ollama の接続先と
 * 模型名を設定し、つながるかを確かめる。自動タグ付けの機能の入切は AutoTaggingSection が持ち、
 * ここでは扱わない（docs/design-docs/auto-tagging.md）。
 */
export default function OllamaSection() {
  const text = t.settings.ollama;
  const toast = useToast();
  const [settings, setSettings] = useState<OllamaSettings | null>(null);
  const [draft, setDraft] = useState<OllamaSettings | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  const [check, setCheck] = useState<CheckState>({ kind: "idle" });
  const controller = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    try {
      const next = await getOllamaSettings(abort.signal);
      if (abort.signal.aborted) return;
      setSettings(next);
      setDraft(next);
      setLoadError(null);
    } catch (failure) {
      if (isAborted(failure)) return;
      setLoadError(errorText(failure));
    }
  }, []);

  useEffect(() => {
    void load();
    return () => controller.current?.abort();
  }, [load]);

  const dirty =
    draft !== null &&
    settings !== null &&
    (draft.endpoint !== settings.endpoint || draft.model !== settings.model);

  /** persist は下書きを保存する。保存できたら true を返す。 */
  const persist = async (): Promise<boolean> => {
    if (draft === null) return false;
    setSaveError(null);
    try {
      const saved = await updateOllamaSettings(draft);
      setSettings(saved);
      setDraft(saved);
      return true;
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
      return false;
    }
  };

  const save = async () => {
    if (draft === null || saving) return;
    setSaving(true);
    try {
      if (await persist()) toast(text.saved);
    } finally {
      setSaving(false);
    }
  };

  // サーバーは保存した接続先でしか確かめないので、編集中の値があれば先に保存する。
  const runCheck = async () => {
    if (draft === null || check.kind === "checking" || saving) return;
    if (dirty) {
      setSaving(true);
      const saved = await persist().finally(() => setSaving(false));
      if (!saved) return;
    }
    setCheck({ kind: "checking" });
    try {
      const result = await checkOllama();
      setCheck(
        result.available
          ? { kind: "ok" }
          : { kind: "failed", message: text.checkFailed(result.message ?? "") },
      );
    } catch (failure) {
      setCheck({ kind: "failed", message: text.checkFailed(errorText(failure)) });
    }
  };

  const update = (change: Partial<OllamaSettings>) => {
    setDraft((current) => (current === null ? current : { ...current, ...change }));
    setSaveError(null);
  };

  if (draft === null) {
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

  return (
    <PageSection title={text.heading} description={text.description}>
      <FormRow
        label={text.endpoint}
        description={text.endpointHint}
        htmlFor="ollama-endpoint"
      >
        <Input
          id="ollama-endpoint"
          type="url"
          className="sm:w-3xs lg:w-sm"
          value={draft.endpoint}
          onChange={(event) => update({ endpoint: event.target.value })}
        />
      </FormRow>
      <FormRow label={text.model} description={text.modelHint} htmlFor="ollama-model">
        <Input
          id="ollama-model"
          className="sm:w-3xs lg:w-sm"
          value={draft.model}
          onChange={(event) => update({ model: event.target.value })}
        />
      </FormRow>
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            onClick={() => void save()}
            aria-disabled={saving || undefined}
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
    </PageSection>
  );
}

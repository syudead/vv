import {
  CircleAlert,
  CircleCheck,
  ExternalLink,
  LoaderCircle,
  TriangleAlert,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  checkComfyUI,
  type ComfyUIMissingItem,
  type ComfyUISettings,
  getComfyUISettings,
  isAborted,
  updateComfyUISettings,
  type UpdateComfyUISettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Field, FieldContent, FieldTitle } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Label } from "../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../ui/shadcn/radio-group";
import { Skeleton } from "../ui/shadcn/skeleton";
import { useToast } from "../ui/Toast";
import { COMFYUI_GUIDE_URL } from "./docsLinks";

type Provider = UpdateComfyUISettings["provider"];

const providerOrder: readonly Provider[] = ["self_hosted", "cloud"];

/** Draft は画面で編集中の値である。`apiKey` は入力欄の値で、保存済みのキーは入らない。 */
interface Draft {
  provider: Provider;
  url: string;
  apiKey: string;
}

type CheckState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "ok"; provider: Provider }
  | { kind: "failed"; message: UiText }
  | { kind: "missing"; items: ComfyUIMissingItem[] };

/** draftOf は保存済みの値から下書きを作る。何も保存されていなければ自前の ComfyUI を選ぶ。 */
function draftOf(settings: ComfyUISettings): Draft {
  return {
    provider: settings.provider ?? "self_hosted",
    url: settings.url ?? "",
    apiKey: "",
  };
}

/**
 * afterSave は保存の応答を受けた後の下書きを返す。送った後に値が変わっていなければ保存後の値に
 * 戻す。変わっていれば送った後の編集を残し、キーの欄は送ったキーのままのときだけ空にする。
 */
function afterSave(current: Draft, sent: Draft, saved: ComfyUISettings): Draft {
  if (
    current.provider === sent.provider &&
    current.url === sent.url &&
    current.apiKey === sent.apiKey
  ) {
    return draftOf(saved);
  }
  return current.apiKey === sent.apiKey ? { ...current, apiKey: "" } : current;
}

/** bodyOf は下書きを保存の要求にする。キーの欄が空なら `apiKey` を省き、保存済みのキーを残す。 */
function bodyOf(draft: Draft): UpdateComfyUISettings {
  const body: UpdateComfyUISettings = { provider: draft.provider };
  if (draft.provider === "self_hosted") body.url = draft.url;
  if (draft.apiKey !== "") body.apiKey = draft.apiKey;
  return body;
}

/** modelsFirst は足りないものをモデルファイル、ノードの順に並べる（ui-design.md Missing files）。 */
function modelsFirst(items: readonly ComfyUIMissingItem[]): ComfyUIMissingItem[] {
  return [
    ...items.filter((item) => item.kind === "model"),
    ...items.filter((item) => item.kind === "node"),
  ];
}

/**
 * ComfyUISection は設定の「ComfyUI」区画である。動画の1コマをサムネイルへ編集する ComfyUI
 * の接続先（自前か Comfy Cloud か）を保存し、つながるかと、vv のワークフローに足りないノード
 * とモデルファイルを確かめる。API キーは書くだけで、画面には戻らない
 * （specs/045-comfyui-thumbnails/ui-design.md、research.md R-9）。
 */
export default function ComfyUISection() {
  const text = t.settings.comfyui;
  const toast = useToast();
  const [settings, setSettings] = useState<ComfyUISettings | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [saving, setSaving] = useState(false);
  const [forgetting, setForgetting] = useState(false);
  const [saveError, setSaveError] = useState<UiText | null>(null);
  const [check, setCheck] = useState<CheckState>({ kind: "idle" });
  const controller = useRef<AbortController | null>(null);
  // 値を変えるたびに進める。テストは始めた時の値と同じ間だけ結果を出す。
  const edits = useRef(0);

  const load = useCallback(async () => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    setLoadError(null);
    try {
      const next = await getComfyUISettings(abort.signal);
      if (abort.signal.aborted) return;
      setSettings(next);
      setDraft(draftOf(next));
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
    (settings.provider === undefined ||
      draft.provider !== settings.provider ||
      (draft.provider === "self_hosted" && draft.url !== (settings.url ?? "")) ||
      draft.apiKey !== "");

  const busy = saving || forgetting;

  /**
   * persist は下書き sent から作った要求を保存する。保存できたら保存後の値を受け取り、入力欄の
   * キーを空に戻す。保存の間に変えた値は残す（afterSave）。
   */
  const persist = async (sent: Draft, body: UpdateComfyUISettings): Promise<boolean> => {
    setSaveError(null);
    try {
      const saved = await updateComfyUISettings(body);
      setSettings(saved);
      setDraft((current) =>
        current === null ? current : afterSave(current, sent, saved),
      );
      return true;
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
      return false;
    }
  };

  const save = async () => {
    if (draft === null || busy) return;
    setSaving(true);
    try {
      if (await persist(draft, bodyOf(draft))) toast(text.saved);
    } finally {
      setSaving(false);
    }
  };

  // 自前の ComfyUI のまま残った Cloud のキーを、今の値と空のキーで保存して消す。
  const forgetKey = async () => {
    if (draft === null || busy) return;
    setForgetting(true);
    try {
      if (await persist(draft, { ...bodyOf(draft), apiKey: "" }))
        toast(text.keyForgotten);
    } finally {
      setForgetting(false);
    }
  };

  // サーバーは保存した接続先でしか確かめないので、編集中の値があれば先に保存する。
  const runCheck = async () => {
    if (draft === null || check.kind === "checking" || busy) return;
    const edit = edits.current;
    let provider: Provider = settings?.provider ?? draft.provider;
    if (dirty) {
      setSaving(true);
      const saved = await persist(draft, bodyOf(draft)).finally(() => setSaving(false));
      // 保存の間に値が変わったら、保存した値を確かめても今の値には当たらない。
      if (!saved || edit !== edits.current) return;
      provider = draft.provider;
    }
    setCheck({ kind: "checking" });
    try {
      const result = await checkComfyUI();
      if (edit !== edits.current) return;
      if (result.available) {
        setCheck({ kind: "ok", provider });
      } else if (result.missing !== undefined && result.missing.length > 0) {
        setCheck({ kind: "missing", items: modelsFirst(result.missing) });
      } else {
        setCheck({ kind: "failed", message: text.checkFailed(result.message ?? "") });
      }
    } catch (failure) {
      if (edit !== edits.current) return;
      setCheck({ kind: "failed", message: text.checkFailed(errorText(failure)) });
    }
  };

  // 値を変えたら前のテスト結果は今の値に当たらないので消す（ui-design.md States）。
  // 確かめている途中なら、その結果も捨てて次のテストを受け付ける。
  const update = (change: Partial<Draft>) => {
    edits.current += 1;
    setDraft((current) => (current === null ? current : { ...current, ...change }));
    setSaveError(null);
    setCheck({ kind: "idle" });
  };

  const description = (
    <>
      {text.description}{" "}
      <a
        href={COMFYUI_GUIDE_URL}
        target="_blank"
        rel="noreferrer"
        className="inline-flex items-center gap-1 text-primary underline underline-offset-4"
      >
        {text.guide}
        <ExternalLink aria-hidden="true" className="size-3" />
        <span className="sr-only">{text.opensInNewTab}</span>
      </a>
    </>
  );

  if (draft === null || settings === null) {
    return (
      <PageSection title={text.heading} description={description}>
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

  const keyStillSaved =
    settings.provider === "self_hosted" &&
    draft.provider === "self_hosted" &&
    settings.apiKeySet;

  return (
    <PageSection title={text.heading} description={description}>
      <Field data-slot="form-row" orientation="responsive" className="sm:gap-6">
        <FieldContent>
          <FieldTitle id="comfyui-provider-label">{text.provider}</FieldTitle>
        </FieldContent>
        <div data-slot="form-row-control" className="flex sm:shrink-0 sm:justify-end">
          <RadioGroup
            name="comfyui-provider"
            value={draft.provider}
            onValueChange={(value) => update({ provider: value as Provider })}
            aria-labelledby="comfyui-provider-label"
            className="sm:w-sm"
          >
            {providerOrder.map((provider) => {
              const itemId = `comfyui-provider-${provider}`;
              const hintId = `comfyui-provider-hint-${provider}`;
              return (
                <div key={provider} className="flex min-w-0 items-start gap-3">
                  <RadioGroupItem
                    id={itemId}
                    value={provider}
                    aria-describedby={hintId}
                    className="mt-0.5"
                  />
                  <div className="flex min-w-0 flex-col gap-0.5">
                    <Label htmlFor={itemId} className="font-normal">
                      {text.providers[provider].label}
                    </Label>
                    <p id={hintId} className="text-sm text-muted-foreground">
                      {text.providers[provider].hint}
                    </p>
                  </div>
                </div>
              );
            })}
          </RadioGroup>
        </div>
      </Field>
      {draft.provider === "self_hosted" ? (
        <FormRow label={text.url} description={text.urlHint} htmlFor="comfyui-url">
          <Input
            id="comfyui-url"
            type="url"
            className="sm:w-sm"
            value={draft.url}
            onChange={(event) => update({ url: event.target.value })}
          />
        </FormRow>
      ) : (
        <FormRow
          label={text.apiKey}
          description={settings.apiKeySet ? text.apiKeySavedHint : text.apiKeyHint}
          htmlFor="comfyui-api-key"
        >
          <Input
            id="comfyui-api-key"
            type="password"
            autoComplete="off"
            className="sm:w-sm"
            value={draft.apiKey}
            onChange={(event) => update({ apiKey: event.target.value })}
          />
        </FormRow>
      )}
      {keyStillSaved && (
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm text-muted-foreground">{text.keyStillSaved}</p>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => void forgetKey()}
            aria-disabled={busy || undefined}
          >
            {forgetting && (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            )}
            {forgetting ? text.forgetting : text.forgetKey}
          </Button>
        </div>
      )}
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" onClick={() => void save()} aria-disabled={busy || undefined}>
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
            aria-disabled={check.kind === "checking" || busy || undefined}
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
            {text.checkOk[check.provider]}
          </p>
        )}
        {check.kind === "failed" && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{check.message}</AlertTitle>
          </Alert>
        )}
        {check.kind === "missing" && (
          <Alert variant="warning">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{text.missingHeading}</AlertTitle>
            <ul className="col-start-2 flex flex-col gap-1 text-foreground">
              {check.items.map((item) => (
                <li
                  key={`${item.kind}:${item.name}`}
                  className="flex flex-wrap items-baseline gap-x-2"
                >
                  <span className="font-mono text-sm break-all">{item.name}</span>
                  {item.kind === "model" ? (
                    <>
                      {item.folder !== undefined && (
                        <span className="text-muted-foreground">
                          {text.putIn(item.folder)}
                        </span>
                      )}
                      {item.source !== undefined && (
                        <Button asChild size="sm" variant="link" className="h-auto p-0">
                          <a href={item.source} target="_blank" rel="noreferrer">
                            {text.download}
                            <span className="sr-only">
                              {` ${item.name} ${text.opensInNewTab}`}
                            </span>
                          </a>
                        </Button>
                      )}
                    </>
                  ) : (
                    <span className="text-muted-foreground">{text.updateForNode}</span>
                  )}
                </li>
              ))}
            </ul>
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

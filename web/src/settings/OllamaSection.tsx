import { CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  checkOllama,
  getOllamaSettings,
  isAborted,
  listOllamaModels,
  type OllamaSettings,
  updateOllamaSettings,
} from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FormRow } from "../ui/patterns/form-row";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Combobox, type ComboboxOption } from "../ui/shadcn/combobox";
import { Input } from "../ui/shadcn/input";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";

type CheckState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "ok"; both: boolean }
  | { kind: "failed"; message: UiText };

type ModelList =
  | { status: "loading" }
  | { status: "ready"; names: string[] }
  | { status: "failed"; reason: UiText };

/** Combobox values cannot be empty, so None needs a sentinel; a model name never has a space. */
const noModel = " none";

/** Reread the names each time a picker opens, since models are pulled outside VVMDM. */
function useOllamaModels(): { models: ModelList; reload: (open: boolean) => void } {
  const [models, setModels] = useState<ModelList>({ status: "loading" });
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);

  const reload = useCallback((open: boolean) => {
    if (!open) return;
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    setModels({ status: "loading" });
    listOllamaModels(abort.signal).then(
      (list) => {
        if (!abort.signal.aborted)
          setModels({ status: "ready", names: list.items.map((item) => item.name) });
      },
      (failure: unknown) => {
        if (!isAborted(failure))
          setModels({ status: "failed", reason: errorText(failure) });
      },
    );
  }, []);
  return { models, reload };
}

function ModelPicker({
  id,
  label,
  value,
  onChange,
  models,
  onOpenChange,
  allowNone,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  models: ModelList;
  onOpenChange: (open: boolean) => void;
  allowNone: boolean;
}) {
  const text = t.settings.ollama;
  const names = models.status === "ready" ? models.names : [];
  const options: ComboboxOption[] = [
    ...(allowNone ? [{ value: noModel, label: text.none }] : []),
    ...names.map((name) => ({ value: name, label: name })),
  ];
  const status =
    models.status === "loading" ? (
      <>
        <Spinner aria-hidden="true" role="presentation" />
        {text.modelsLoading}
      </>
    ) : models.status === "failed" ? (
      text.modelsFailed(models.reason)
    ) : undefined;
  return (
    <Combobox
      id={id}
      aria-label={label}
      className="sm:w-3xs lg:w-sm"
      options={options}
      value={value}
      onValueChange={(next) => onChange(next === noModel ? "" : next)}
      onOpenChange={onOpenChange}
      placeholder={text.none}
      searchPlaceholder={text.modelSearch}
      emptyText={text.modelsEmpty}
      status={status}
      createLabel={text.useTyped}
    />
  );
}

function sameSettings(a: OllamaSettings, b: OllamaSettings): boolean {
  return (
    a.endpoint === b.endpoint &&
    a.model === b.model &&
    a.generationModel === b.generationModel
  );
}

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
  const { models, reload } = useOllamaModels();

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

  const dirty = draft !== null && settings !== null && !sameSettings(draft, settings);

  const persist = async (): Promise<OllamaSettings | null> => {
    if (draft === null) return null;
    setSaveError(null);
    try {
      const saved = await updateOllamaSettings(draft);
      setSettings(saved);
      setDraft(saved);
      return saved;
    } catch (failure) {
      setSaveError(text.saveFailed(errorText(failure)));
      return null;
    }
  };

  const save = async () => {
    if (draft === null || saving) return;
    setSaving(true);
    try {
      if ((await persist()) !== null) toast(text.saved);
    } finally {
      setSaving(false);
    }
  };

  // Save draft changes first because the server tests only stored settings.
  const runCheck = async () => {
    if (draft === null || settings === null || check.kind === "checking" || saving)
      return;
    let tested = settings;
    if (dirty) {
      setSaving(true);
      const saved = await persist().finally(() => setSaving(false));
      if (saved === null) return;
      tested = saved;
    }
    setCheck({ kind: "checking" });
    try {
      const result = await checkOllama();
      if (result.available) {
        setCheck({ kind: "ok", both: tested.generationModel !== "" });
      } else if (result.reason === "generation_model_missing") {
        setCheck({
          kind: "failed",
          message: text.generationModelMissing(tested.generationModel),
        });
      } else {
        setCheck({ kind: "failed", message: text.checkFailed(result.message ?? "") });
      }
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
        <ModelPicker
          id="ollama-model"
          label={text.model}
          value={draft.model}
          onChange={(model) => update({ model })}
          models={models}
          onOpenChange={reload}
          allowNone={false}
        />
      </FormRow>
      <FormRow
        label={text.generationModel}
        description={text.generationModelHint}
        htmlFor="ollama-generation-model"
      >
        <ModelPicker
          id="ollama-generation-model"
          label={text.generationModel}
          value={draft.generationModel}
          onChange={(generationModel) => update({ generationModel })}
          models={models}
          onOpenChange={reload}
          allowNone
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
            {check.both ? text.checkOkBoth : text.checkOk}
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

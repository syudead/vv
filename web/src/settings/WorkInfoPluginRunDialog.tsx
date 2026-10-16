import { CircleAlert } from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";

import {
  getWorkInfoPluginLastRun,
  type WorkInfoPlugin,
  type WorkInfoPluginRun,
} from "../api/workInfoPlugins";
import { errorText, formatDateTime, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { FactList } from "../ui/patterns/fact-list";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Skeleton } from "../ui/shadcn/skeleton";
import { pluginName, runFailureText, unixDate } from "./workInfoPluginRows";

const text = () => t.settings.workInfoPlugins.runDialog;

/** Loaded は読み込みの結果である。run が null なら今のモジュールの実行が残っていない。 */
type Loaded =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "loaded"; run: WorkInfoPluginRun | null };

function RunFacts({ run }: { run: WorkInfoPluginRun }) {
  const operation = t.settings.workInfoPlugins.operations[run.operation];
  const result =
    run.failure === undefined ? (
      text().finished
    ) : (
      <span className="text-destructive">
        {text().failed(runFailureText(run.failure))}
      </span>
    );
  return (
    <FactList
      facts={[
        { id: "ran", term: text().ran, value: formatDateTime(unixDate(run.ranAt)) },
        { id: "did", term: text().did, value: operation },
        { id: "result", term: text().result, value: result },
      ]}
    />
  );
}

function RunOutput({ run }: { run: WorkInfoPluginRun }) {
  const labelId = useId();
  if (run.stderr === "")
    return (
      <p role="status" className="text-muted-foreground">
        {text().wroteNothing}
      </p>
    );
  return (
    <>
      {run.truncated && (
        <p role="status" className="inline-flex items-center gap-1 text-warning">
          <CircleAlert aria-hidden="true" className="size-4 shrink-0" />
          {text().truncated}
        </p>
      )}
      <span id={labelId} className="sr-only">
        {text().output}
      </span>
      {/* 長い行は折り返してダイアログを広げず、高い出力はこの区画の中だけで動かす。 */}
      <pre
        role="region"
        aria-labelledby={labelId}
        tabIndex={0}
        className="max-h-run-output overflow-y-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap"
      >
        {run.stderr}
      </pre>
    </>
  );
}

function RunBody({ loaded, onRetry }: { loaded: Loaded; onRetry: () => void }) {
  if (loaded.kind === "loading")
    return (
      <div role="status" aria-label={text().loading} className="flex flex-col gap-2">
        <Skeleton className="h-5" />
        <Skeleton className="h-5" />
        <Skeleton className="h-5" />
        <Skeleton className="h-16" />
      </div>
    );
  if (loaded.kind === "failed")
    return (
      <ErrorState
        title={text().loadFailed(loaded.reason)}
        retryLabel={t.common.retry}
        onRetry={onRetry}
      />
    );
  if (loaded.run === null)
    return (
      <p role="status" className="text-muted-foreground">
        {text().noRun}
      </p>
    );
  return (
    <>
      <RunFacts run={loaded.run} />
      <RunOutput run={loaded.run} />
    </>
  );
}

/**
 * WorkInfoPluginRunDialog はプラグインの直近の実行が標準エラーに書いたものを見せる。開くたびに
 * `GET …/{id}/last-run` を読むので、設定画面を開いている間に終わった実行も出る。秘密の設定の
 * 値はサーバーが `[secret]` に置き換えてある（specs/056-plugin-tooling/ui-design.md
 * 「Last run dialog」）。
 */
export default function WorkInfoPluginRunDialog({
  plugin,
  onClose,
}: {
  plugin: WorkInfoPlugin;
  onClose: () => void;
}) {
  const [loaded, setLoaded] = useState<Loaded>({ kind: "loading" });
  const controller = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    controller.current?.abort();
    const current = new AbortController();
    controller.current = current;
    setLoaded({ kind: "loading" });
    try {
      const run = await getWorkInfoPluginLastRun(plugin.id, current.signal);
      if (!current.signal.aborted) setLoaded({ kind: "loaded", run });
    } catch (failure) {
      if (!current.signal.aborted)
        setLoaded({ kind: "failed", reason: errorText(failure) });
    }
  }, [plugin.id]);

  useEffect(() => {
    void load();
    return () => controller.current?.abort();
  }, [load]);

  return (
    <FormDialog
      open
      size="wide"
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={text().title(pluginName(plugin))}
      description={text().description}
      submitLabel={text().close}
      onSubmit={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <RunBody loaded={loaded} onRetry={() => void load()} />
    </FormDialog>
  );
}

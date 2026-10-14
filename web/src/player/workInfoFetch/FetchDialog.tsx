import { CircleCheck } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed, type Video } from "../../api/client";
import { type DetailMark, detailMark } from "../../api/useVideoDetail";
import {
  applyWorkInfoFetch,
  fetchWorkInfo,
  searchWorkInfo,
  type SkippedTag,
  type WorkInfoCandidate,
  type WorkInfoField,
  type WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { t } from "../../i18n";
import { FormDialog } from "../../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../../ui/shadcn/alert";
import { Spinner } from "../../ui/shadcn/spinner";
import { useToast } from "../../ui/Toast";
import CompareStep, { type Fetched } from "./CompareStep";
import {
  initialQuery,
  initialSelection,
  orderSkipped,
  type RunOperation,
  runFor,
} from "./fetchRules";
import FindStep, { type RunFailure, type Running } from "./FindStep";

/** 一覧を読み直す失敗（プラグインが承認を外されたか消された）。 */
const PLUGIN_GONE = new Set(["plugin_not_approved", "not_found"]);

type Step = "find" | "compare" | "result";

function ResultStep({ skipped }: { skipped: readonly SkippedTag[] }) {
  const text = t.player.workInfoFetch;
  return (
    <>
      <Alert variant="success" role="status">
        <CircleCheck aria-hidden="true" />
        <AlertTitle>{text.resultTitle}</AlertTitle>
      </Alert>
      <ul className="flex flex-col gap-1 text-sm">
        {skipped.map((tag) => (
          <li key={tag.name} className="break-words">
            <span className="font-medium">{tag.name}</span>
            {` — ${text.skipped[tag.reason]}`}
          </li>
        ))}
      </ul>
    </>
  );
}

function busyLabel(label: string): ReactNode {
  return (
    <>
      <Spinner aria-hidden="true" role={undefined} />
      {label}
    </>
  );
}

/**
 * useFetchFlow は取得のダイアログの状態と動きである（ui-design「Fetch dialog」の図）。検索と取得は
 * 1 つずつ走らせ、ダイアログを閉じれば答えを捨てる。反映は動画を変える唯一の要求で、送っている
 * 間はダイアログを閉じない。
 */
function useFetchFlow({
  video,
  plugins,
  onReload,
  onApplied,
  onClose,
}: {
  video: Video;
  plugins: readonly WorkInfoPlugin[];
  onReload: () => Promise<void>;
  onApplied: (video: Video, mark: DetailMark) => void;
  onClose: () => void;
}) {
  const toast = useToast();
  const [pluginId, setPluginId] = useState(plugins[0]?.id);
  const plugin = plugins.find((item) => item.id === pluginId) ?? plugins[0];
  const [query, setQuery] = useState(() => initialQuery(plugins[0], video.title));
  const [notUrl, setNotUrl] = useState(false);
  const [candidates, setCandidates] = useState<WorkInfoCandidate[] | null>(null);
  const [running, setRunning] = useState<Running | null>(null);
  const [failure, setFailure] = useState<RunFailure | null>(null);
  const [step, setStep] = useState<Step>("find");
  const [fetched, setFetched] = useState<Fetched | null>(null);
  const [selection, setSelection] = useState<WorkInfoField[]>([]);
  const [stale, setStale] = useState(false);
  const [applyFailure, setApplyFailure] = useState<unknown>(null);
  const [applying, setApplying] = useState(false);
  const [skipped, setSkipped] = useState<SkippedTag[]>([]);
  const controller = useRef<AbortController | null>(null);
  const latestVideo = useRef(video);
  latestVideo.current = video;

  useEffect(() => () => controller.current?.abort(), []);

  const begin = () => {
    controller.current?.abort();
    controller.current = new AbortController();
    return controller.current.signal;
  };

  const showCompare = (next: Fetched, result: Fetched["result"]) => {
    setFetched({ ...next, result });
    setSelection(initialSelection(latestVideo.current, result));
    setStale(false);
    setApplyFailure(null);
    setStep("compare");
  };

  const run = async (
    target: WorkInfoPlugin,
    operation: RunOperation,
    input: string,
    fromSearch: boolean,
  ) => {
    const signal = begin();
    setRunning({ operation, url: fromSearch ? input : undefined });
    setFailure(null);
    setNotUrl(false);
    try {
      if (operation === "search") {
        setCandidates(await searchWorkInfo(target.id, input.trim(), signal));
        return;
      }
      const url = input.trim();
      const result = await fetchWorkInfo(video.id, target.id, url, signal);
      if (signal.aborted) return;
      showCompare({ url, plugin: target, fromSearch, result }, result);
    } catch (error) {
      if (isAborted(error) || signal.aborted) return;
      setFailure({ operation, error, plugin: target });
      setStep("find");
      if (error instanceof RequestFailed && PLUGIN_GONE.has(error.code ?? ""))
        void onReload();
    } finally {
      if (!signal.aborted) setRunning(null);
    }
  };

  const apply = async () => {
    if (fetched === null) return;
    const signal = begin();
    setApplying(true);
    setApplyFailure(null);
    const mark = detailMark();
    try {
      const applied = await applyWorkInfoFetch(
        video.id,
        fetched.result.fetchId,
        selection,
        signal,
      );
      onApplied(applied.video, mark);
      if (applied.skippedTags.length === 0) {
        toast(t.player.workInfoFetch.applied);
        onClose();
        return;
      }
      setSkipped(orderSkipped(applied.skippedTags));
      setStep("result");
    } catch (error) {
      if (isAborted(error) || signal.aborted) return;
      if (error instanceof RequestFailed && error.code === "work_info_fetch_stale")
        setStale(true);
      else setApplyFailure(error);
    } finally {
      if (!signal.aborted) setApplying(false);
    }
  };

  const submit = () => {
    if (step === "result") return onClose();
    if (step === "compare") {
      if (!applying && !stale && running === null && selection.length > 0) void apply();
      return;
    }
    if (plugin === undefined || running !== null || query.trim() === "") return;
    const operation = runFor(plugin, query);
    if (operation === "notUrl") setNotUrl(true);
    else void run(plugin, operation, query, false);
  };

  return {
    plugin,
    query,
    notUrl,
    candidates,
    running,
    failure,
    step,
    fetched,
    selection,
    stale,
    applyFailure,
    applying,
    skipped,
    submit,
    choosePlugin: (id: number) => {
      setPluginId(id);
      setCandidates(null);
      setFailure(null);
      setNotUrl(false);
    },
    changeQuery: (next: string) => {
      setQuery(next);
      setNotUrl(false);
    },
    choose: (url: string) => {
      if (plugin !== undefined && running === null) void run(plugin, "fetch", url, true);
    },
    toggle: (field: WorkInfoField, on: boolean) =>
      setSelection((current) =>
        on ? [...new Set([...current, field])] : current.filter((item) => item !== field),
      ),
    back: () => setStep("find"),
    fetchAgain: () => {
      // 同じプラグインと URL で取り直す。プラグインが承認を外されていれば、その失敗が
      // 見つける段に出る。
      if (fetched !== null && running === null)
        void run(fetched.plugin, "fetch", fetched.url, fetched.fromSearch);
    },
    cancel: () => controller.current?.abort(),
  };
}

type Flow = ReturnType<typeof useFetchFlow>;

function submitLabel(flow: Flow): ReactNode {
  const text = t.player.workInfoFetch;
  if (flow.step === "result") return text.done;
  if (flow.step === "compare")
    return flow.applying ? busyLabel(text.applying) : text.apply;
  const operation =
    flow.plugin === undefined ? "search" : runFor(flow.plugin, flow.query);
  if (flow.running !== null && flow.running.url === undefined)
    return busyLabel(
      flow.running.operation === "search" ? text.searching : text.fetching,
    );
  return operation === "search" ? text.search : text.fetch;
}

function submitDisabled(flow: Flow): boolean {
  if (flow.step === "result") return false;
  if (flow.step === "compare")
    return flow.selection.length === 0 || flow.stale || flow.running !== null;
  return flow.plugin === undefined || flow.running !== null || flow.query.trim() === "";
}

/**
 * FetchDialog は取得のダイアログである（specs/049-work-info-plugins/ui-design.md「Fetch
 * dialog」）。見つける段（検索の候補を含む）、比較の段、付けなかったタグを知らせる結果の段を
 * 1 つの窓で切り替える。反映が成功するまで動画は変わらない。
 */
export default function FetchDialog(props: {
  video: Video;
  plugins: readonly WorkInfoPlugin[];
  onReload: () => Promise<void>;
  onApplied: (video: Video, mark: DetailMark) => void;
  onClose: () => void;
}) {
  const { video, plugins, onClose } = props;
  const flow = useFetchFlow(props);
  const text = t.player.workInfoFetch;
  const queryRef = useRef<HTMLInputElement | null>(null);
  return (
    <FormDialog
      open
      size="wide"
      onOpenChange={(open) => {
        if (open || flow.applying) return;
        flow.cancel();
        onClose();
      }}
      title={text.title}
      description={text.description}
      submitLabel={submitLabel(flow)}
      cancelLabel={flow.step === "result" ? undefined : t.common.cancel}
      pending={flow.applying}
      submitDisabled={submitDisabled(flow)}
      initialFocus={queryRef}
      onSubmit={(event) => {
        event.preventDefault();
        flow.submit();
      }}
    >
      {flow.step === "find" && (
        <FindStep
          plugins={plugins}
          plugin={flow.plugin}
          query={flow.query}
          notUrl={flow.notUrl}
          running={flow.running}
          failure={flow.failure}
          candidates={flow.candidates}
          queryRef={queryRef}
          onPlugin={flow.choosePlugin}
          onQuery={flow.changeQuery}
          onChoose={flow.choose}
        />
      )}
      {flow.step === "compare" && flow.fetched !== null && (
        <CompareStep
          video={video}
          fetched={flow.fetched}
          selection={flow.selection}
          locked={flow.applying}
          stale={flow.stale}
          refetching={flow.running !== null}
          failure={flow.applyFailure}
          onToggle={flow.toggle}
          onBack={flow.back}
          onFetchAgain={flow.fetchAgain}
        />
      )}
      {flow.step === "result" && <ResultStep skipped={flow.skipped} />}
    </FormDialog>
  );
}

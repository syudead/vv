import { CircleCheck } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed, type Video } from "../../api/client";
import { type DetailMark, detailMark } from "../../api/useVideoDetail";
import {
  applyWorkInfoFetch,
  fetchWorkInfo,
  searchWorkInfo,
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
  canFetch,
  type FindBy,
  initialQuery,
  initialSelection,
  type RunOperation,
  runFor,
  selectableTags,
  type SkippedLine,
  skippedLines,
  withField,
} from "./fetchRules";
import FindStep, { type RunFailure, type Running } from "./FindStep";

/** 一覧を読み直す失敗（プラグインが承認を外されたか消された）。 */
const PLUGIN_GONE = new Set(["plugin_not_approved", "not_found"]);

type Step = "find" | "compare" | "result";

function ResultStep({ lines }: { lines: readonly SkippedLine[] }) {
  const text = t.player.workInfoFetch;
  return (
    <>
      <Alert variant="success" role="status">
        <CircleCheck aria-hidden="true" />
        <AlertTitle>{text.resultTitle}</AlertTitle>
      </Alert>
      <ul className="flex flex-col gap-1 text-sm">
        {lines.map((line) => (
          <li key={line.key} className="break-words">
            {text.skippedLabel(line.label)}
            <span className="font-medium">{line.value}</span>
            {` — ${line.reason}`}
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
  const [by, setBy] = useState<FindBy>("words");
  const [query, setQuery] = useState(() => initialQuery(plugins[0], "words", video));
  const [candidates, setCandidates] = useState<WorkInfoCandidate[] | null>(null);
  if (plugin?.id !== pluginId) {
    // 選んでいたプラグインが一覧から消えた（承認を外されたか消された）ので、先頭へ移る。
    // 候補は消えたプラグインの答えなので、プラグインを選び直したときと同じく捨てる。
    setPluginId(plugin?.id);
    setCandidates(null);
  }
  const [running, setRunning] = useState<Running | null>(null);
  const [failure, setFailure] = useState<RunFailure | null>(null);
  const [step, setStep] = useState<Step>("find");
  const [fetched, setFetched] = useState<Fetched | null>(null);
  const [selection, setSelection] = useState<WorkInfoField[]>([]);
  // 選んだタグの名前。1 つでも選ばれていれば selection が `tags` を含む（056 R-7）。
  const [tags, setTags] = useState<string[]>([]);
  const [stale, setStale] = useState(false);
  const [applyFailure, setApplyFailure] = useState<unknown>(null);
  const [applying, setApplying] = useState(false);
  const [skipped, setSkipped] = useState<SkippedLine[]>([]);
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
    setTags(selectableTags(latestVideo.current, result));
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
    setRunning({ operation, id: fromSearch ? input : undefined });
    setFailure(null);
    try {
      if (operation === "search") {
        setCandidates(await searchWorkInfo(target.id, input.trim(), signal));
        return;
      }
      // 入れた文字列は URL の形でも作品の ID としてそのまま送る（親 Issue 1232 の要件 14）。
      const source = input.trim();
      const result = await fetchWorkInfo(video.id, target.id, source, signal);
      if (signal.aborted) return;
      showCompare({ source, plugin: target, fromSearch, result }, result);
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
        selection.includes("tags") ? tags : undefined,
        signal,
      );
      onApplied(applied.video, mark);
      const lines = skippedLines(applied.skippedTags, applied.skipped);
      if (lines.length === 0) {
        toast(t.player.workInfoFetch.applied);
        onClose();
        return;
      }
      setSkipped(lines);
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
    void run(plugin, runFor(plugin, by), query, false);
  };

  return {
    plugin,
    by,
    query,
    candidates,
    running,
    failure,
    step,
    fetched,
    selection,
    tags,
    stale,
    applyFailure,
    applying,
    skipped,
    submit,
    choosePlugin: (id: number) => {
      setPluginId(id);
      setCandidates(null);
      setFailure(null);
    },
    chooseBy: (next: FindBy) => {
      // 入れたままの最初の値は、語と ID で違う（題名と品番）ので入れ替える。打った語は残す。
      if (plugin !== undefined && query === initialQuery(plugin, by, latestVideo.current))
        setQuery(initialQuery(plugin, next, latestVideo.current));
      setBy(next);
      setCandidates(null);
      setFailure(null);
    },
    changeQuery: setQuery,
    choose: (id: string) => {
      // 候補を選ぶことはその作品 ID の取得なので、取得できないプラグインでは走らせない。
      if (plugin !== undefined && canFetch(plugin) && running === null)
        void run(plugin, "fetch", id, true);
    },
    toggle: (field: WorkInfoField, on: boolean) => {
      // タグの行のチェックボックスは、選べるタグを全部選ぶか全部外す。
      if (field === "tags" && fetched !== null)
        setTags(on ? selectableTags(latestVideo.current, fetched.result) : []);
      setSelection((current) => withField(current, field, on));
    },
    toggleTag: (name: string, on: boolean) => {
      // 1 つも選ばれていなければ、反映の項目からタグを外す。
      const next = on
        ? [...new Set([...tags, name])]
        : tags.filter((item) => item !== name);
      setTags(next);
      setSelection((current) => withField(current, "tags", next.length > 0));
    },
    back: () => setStep("find"),
    fetchAgain: () => {
      // 同じプラグインと作品 ID で取り直す。プラグインが承認を外されていれば、その失敗が
      // 見つける段に出る。
      if (fetched === null || running !== null) return;
      void run(fetched.plugin, "fetch", fetched.source, fetched.fromSearch);
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
  const operation = flow.plugin === undefined ? "search" : runFor(flow.plugin, flow.by);
  if (flow.running !== null && flow.running.id === undefined)
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
 * dialog」）。見つける段（検索の候補を含む）、比較の段、付けなかったタグと飛ばした値を知らせる
 * 結果の段（specs/052-plugin-work-info-fields/ui-design.md「Result」）を
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
          by={flow.by}
          query={flow.query}
          running={flow.running}
          failure={flow.failure}
          candidates={flow.candidates}
          queryRef={queryRef}
          onPlugin={flow.choosePlugin}
          onBy={flow.chooseBy}
          onQuery={flow.changeQuery}
          onChoose={flow.choose}
        />
      )}
      {flow.step === "compare" && flow.fetched !== null && (
        <CompareStep
          video={video}
          fetched={flow.fetched}
          selection={flow.selection}
          tags={flow.tags}
          locked={flow.applying}
          stale={flow.stale}
          refetching={flow.running !== null}
          failure={flow.applyFailure}
          onToggle={flow.toggle}
          onToggleTag={flow.toggleTag}
          onBack={flow.back}
          onFetchAgain={flow.fetchAgain}
        />
      )}
      {flow.step === "result" && <ResultStep lines={flow.skipped} />}
    </FormDialog>
  );
}

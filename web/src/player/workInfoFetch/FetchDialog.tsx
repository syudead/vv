import { CircleCheck, UserRoundSearch } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  isAborted,
  RequestFailed,
  uploadThumbnailImage,
  type Video,
} from "../../api/client";
import { type DetailMark, detailMark } from "../../api/useVideoDetail";
import {
  applyWorkInfoFetch,
  fetchWorkInfo,
  searchWorkInfo,
  type WorkInfoCandidate,
  type WorkInfoFetchApplied,
  type WorkInfoPlugin,
} from "../../api/workInfoPlugins";
import { t } from "../../i18n";
import { FormDialog } from "../../ui/patterns/form-dialog";
import { creditDisplayName } from "../../credits/displayName";
import { Alert, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { useToast } from "../../ui/Toast";
import { encodeComposition } from "../thumbnailUpload/composeImage";
import CompareStep, { type Fetched } from "./CompareStep";
import { type CoverFraming, useCoverFraming } from "./coverFraming";
import {
  canFetch,
  type FindBy,
  initialQuery,
  type RunOperation,
  runFor,
  type SkippedLine,
  skippedLines,
} from "./fetchRules";
import FindStep, { type RunFailure, type Running } from "./FindStep";
import {
  initialWorkSelection,
  workChoice,
  type WorkSelection,
  workSelectionEmpty,
} from "./workSelection";

const PLUGIN_GONE = new Set(["plugin_not_approved", "not_found"]);

type Step = "find" | "compare" | "result";

const EMPTY_SELECTION: WorkSelection = {
  fields: [],
  tags: [],
  cover: null,
  people: [],
  organizations: [],
  urls: [],
  externalIds: [],
  samePeople: [],
};

export function ResultStep({
  lines,
  people = [],
}: {
  lines: readonly SkippedLine[];
  people?: readonly LinkedPerson[];
}) {
  const text = t.player.workInfoFetch;
  return (
    <>
      <Alert variant="success" role="status">
        <CircleCheck aria-hidden="true" />
        <AlertTitle>{lines.length > 0 ? text.resultTitle : text.applied}</AlertTitle>
      </Alert>
      {lines.length > 0 && (
        <ul className="flex flex-col gap-1 text-sm">
          {lines.map((line) => (
            <li key={line.key} className="break-words">
              {text.skippedLabel(line.label)}
              <span className="font-medium">{line.value}</span>
              {` — ${line.reason}`}
            </li>
          ))}
        </ul>
      )}
      {people.length > 0 && <LinkedPeople people={people} />}
    </>
  );
}

export interface LinkedPerson {
  id: number;
  name: string;
}

/** The people the apply linked, each a step from its page's Fetch info, which fills a person from a person plugin. */
function LinkedPeople({ people }: { people: readonly LinkedPerson[] }) {
  const text = t.player.workInfoFetch;
  return (
    <section className="flex flex-col gap-1 text-sm">
      <h3 className="font-medium">{text.linkedTitle}</h3>
      <p className="text-xs text-muted-foreground">{text.linkedNote}</p>
      <ul className="flex flex-wrap gap-1">
        {people.map((person) => (
          <li key={person.id}>
            <Button variant="outline" size="sm" asChild>
              <Link to={`/people/${String(person.id)}?fetch=1`}>
                <UserRoundSearch aria-hidden="true" />
                <span className="max-w-chip-label truncate">
                  {text.fetchPersonInfo(person.name)}
                </span>
              </Link>
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** The linked people as the applied video names them, in the apply's order. */
function linkedPeople(applied: WorkInfoFetchApplied): LinkedPerson[] {
  const people = applied.video.workInfo?.people ?? [];
  return applied.linkedPeople.flatMap((id) => {
    const entry = people.find((item) => item.person.id === id);
    return entry === undefined ? [] : [{ id, name: creditDisplayName(entry.person) }];
  });
}

export function busyLabel(label: string): ReactNode {
  return (
    <>
      <Spinner aria-hidden="true" role={undefined} />
      {label}
    </>
  );
}

/** Upload a cover the page framed after the other fields commit; a failure leaves the thumbnail and reports the cover as skipped, as the server does for a cover it fits. */
async function uploadFramedCover(
  applied: WorkInfoFetchApplied,
  cover: CoverFraming & { image: NonNullable<CoverFraming["image"]> },
  size: string,
  signal: AbortSignal,
): Promise<WorkInfoFetchApplied> {
  try {
    const composed = await encodeComposition(cover.image, cover.aspect, cover.fit);
    const video = await uploadThumbnailImage(
      applied.video.id,
      new File([composed], "thumbnail.png", { type: "image/png" }),
      applied.video.contentVersion ?? "",
      signal,
    );
    return { ...applied, video };
  } catch (error) {
    if (isAborted(error) || signal.aborted) throw error;
    return {
      ...applied,
      skipped: [...applied.skipped, { field: "cover", value: size, reason: "invalid" }],
    };
  }
}

/** Serialize search/fetch and discard responses after closing. Applying is the only mutation and prevents closing until complete. */
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
    // Losing plugin approval invalidates its candidates; return to plugin selection.
    setPluginId(plugin?.id);
    setCandidates(null);
  }
  const [running, setRunning] = useState<Running | null>(null);
  const [failure, setFailure] = useState<RunFailure | null>(null);
  const [step, setStep] = useState<Step>("find");
  const [fetched, setFetched] = useState<Fetched | null>(null);
  const [selection, setSelection] = useState<WorkSelection>(EMPTY_SELECTION);
  const [linked, setLinked] = useState<LinkedPerson[]>([]);
  const [stale, setStale] = useState(false);
  const [applyFailure, setApplyFailure] = useState<unknown>(null);
  const [applying, setApplying] = useState(false);
  const [skipped, setSkipped] = useState<SkippedLine[]>([]);
  const cover = useCoverFraming(
    video,
    step === "compare" ? (fetched?.result ?? null) : null,
    selection.cover,
  );
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
    setSelection(initialWorkSelection(latestVideo.current, result));
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
      // Treat URL-shaped text as an opaque work ID, never as a fetchable URL.
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
    // A cover the page decoded is checked and consumed by the apply, then uploaded with the owner's framing.
    const chosen =
      selection.cover === null ? undefined : fetched.result.covers[selection.cover];
    const framed = chosen === undefined ? null : cover.image;
    try {
      let applied = await applyWorkInfoFetch(
        video.id,
        fetched.result.fetchId,
        workChoice(selection),
        signal,
        framed !== null,
      );
      if (framed !== null && chosen !== undefined)
        applied = await uploadFramedCover(
          applied,
          { ...cover, image: framed },
          t.player.workInfoFetch.coverSize(chosen.width, chosen.height),
          signal,
        );
      onApplied(applied.video, mark);
      const lines = skippedLines(applied.skippedTags, applied.skipped);
      const people = linkedPeople(applied);
      if (lines.length === 0 && people.length === 0) {
        toast(t.player.workInfoFetch.applied);
        onClose();
        return;
      }
      setSkipped(lines);
      setLinked(people);
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
      if (!applying && !stale && running === null && !workSelectionEmpty(selection))
        void apply();
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
    linked,
    stale,
    applyFailure,
    applying,
    skipped,
    cover,
    submit,
    choosePlugin: (id: number) => {
      setPluginId(id);
      setCandidates(null);
      setFailure(null);
    },
    chooseBy: (next: FindBy) => {
      // Switch untouched defaults between title and code, but preserve user-entered text.
      if (plugin !== undefined && query === initialQuery(plugin, by, latestVideo.current))
        setQuery(initialQuery(plugin, next, latestVideo.current));
      setBy(next);
      setCandidates(null);
      setFailure(null);
    },
    changeQuery: setQuery,
    choose: (id: string) => {
      // Selecting a search result requires fetch capability.
      if (plugin !== undefined && canFetch(plugin) && running === null)
        void run(plugin, "fetch", id, true);
    },
    changeSelection: setSelection,
    back: () => setStep("find"),
    fetchAgain: () => {
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
    return workSelectionEmpty(flow.selection) || flow.stale || flow.running !== null;
  return flow.plugin === undefined || flow.running !== null || flow.query.trim() === "";
}

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
          locked={flow.applying}
          stale={flow.stale}
          refetching={flow.running !== null}
          failure={flow.applyFailure}
          cover={flow.cover}
          onChange={flow.changeSelection}
          onBack={flow.back}
          onFetchAgain={flow.fetchAgain}
        />
      )}
      {flow.step === "result" && <ResultStep lines={flow.skipped} people={flow.linked} />}
    </FormDialog>
  );
}

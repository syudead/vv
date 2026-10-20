import { useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "../../../api/client";
import { isCreditGone, type Person } from "../../../api/credits";
import {
  applyPersonPluginFetch,
  fetchPersonWithPlugin,
  type PersonPluginCandidate,
  type PersonPluginFetch,
  type PersonPluginItem,
  searchPersonWithPlugin,
} from "../../../api/personPluginFetches";
import type { WorkInfoPlugin } from "../../../api/workInfoPlugins";
import { t } from "../../../i18n";
import type { SkippedLine } from "../../../player/workInfoFetch/fetchRules";
import type { RunFailure } from "../../../player/workInfoFetch/FindStep";
import { useToast } from "../../../ui/Toast";
import {
  compareRows,
  initialImage,
  initialItems,
  skippedPersonLines,
} from "./personCompareRows";

const PLUGIN_GONE = new Set(["plugin_not_approved", "not_found"]);

export type Step = "find" | "compare" | "result";

export interface CandidateId {
  source: string;
  id: string;
}

export interface PersonFetched {
  result: PersonPluginFetch;
  plugin: WorkInfoPlugin;
  /** The chosen search candidate; absent when the server picked the stored ID. */
  candidate?: CandidateId;
}

export interface PersonRunning {
  operation: "search" | "fetch";
  /** The chosen candidate's key while its fetch runs. */
  key?: string;
}

export function candidateKey(candidate: CandidateId): string {
  return `${candidate.source}\u0000${candidate.id}`;
}

function isMissingId(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "person_external_id_missing";
}

/** One request at a time; responses after closing are dropped, and closing waits for an apply. */
export function usePersonFetchFlow({
  person,
  plugins,
  onReload,
  onApplied,
  onGone,
  onClose,
}: {
  person: Person;
  plugins: readonly WorkInfoPlugin[];
  onReload: () => Promise<void>;
  onApplied: (person: Person) => void;
  onGone: () => void;
  onClose: () => void;
}) {
  const toast = useToast();
  const [pluginId, setPluginId] = useState(plugins[0]?.id);
  const plugin = plugins.find((item) => item.id === pluginId) ?? plugins[0];
  // The page offers a name search on request, or after the server answers person_external_id_missing.
  const [byName, setByName] = useState(false);
  const [missingId, setMissingId] = useState(false);
  const [query, setQuery] = useState(person.name);
  const [candidates, setCandidates] = useState<PersonPluginCandidate[] | null>(null);
  if (plugin?.id !== pluginId) {
    setPluginId(plugin?.id);
    setByName(false);
    setMissingId(false);
    setCandidates(null);
  }
  const [running, setRunning] = useState<PersonRunning | null>(null);
  const [failure, setFailure] = useState<RunFailure | null>(null);
  const [step, setStep] = useState<Step>("find");
  const [fetched, setFetched] = useState<PersonFetched | null>(null);
  const [selection, setSelection] = useState<PersonPluginItem[]>([]);
  const [image, setImage] = useState<number | null>(null);
  const [stale, setStale] = useState(false);
  const [applyFailure, setApplyFailure] = useState<unknown>(null);
  const [applying, setApplying] = useState(false);
  const [skipped, setSkipped] = useState<SkippedLine[]>([]);
  const controller = useRef<AbortController | null>(null);
  const latestPerson = useRef(person);
  latestPerson.current = person;

  useEffect(() => () => controller.current?.abort(), []);

  const begin = () => {
    controller.current?.abort();
    controller.current = new AbortController();
    return controller.current.signal;
  };

  const gone = () => {
    toast(t.credits.personPage.pluginFetch.gone);
    onGone();
    onClose();
  };

  const failed = (error: unknown, operation: PersonRunning["operation"]) => {
    if (isCreditGone(error)) return gone();
    setFailure({ operation, error, plugin });
    setStep("find");
    if (error instanceof RequestFailed && PLUGIN_GONE.has(error.code)) void onReload();
  };

  const search = async (target: WorkInfoPlugin) => {
    const signal = begin();
    setRunning({ operation: "search" });
    setFailure(null);
    try {
      setCandidates(await searchPersonWithPlugin(target.id, query.trim(), signal));
    } catch (error) {
      if (isAborted(error) || signal.aborted) return;
      failed(error, "search");
    } finally {
      if (!signal.aborted) setRunning(null);
    }
  };

  const fetchPerson = async (target: WorkInfoPlugin, candidate?: CandidateId) => {
    const signal = begin();
    setRunning({
      operation: "fetch",
      key: candidate === undefined ? undefined : candidateKey(candidate),
    });
    setFailure(null);
    try {
      const result = await fetchPersonWithPlugin(person.id, target.id, candidate, signal);
      if (signal.aborted) return;
      setFetched({ result, plugin: target, candidate });
      setSelection(initialItems(compareRows(latestPerson.current, result.person, true)));
      setImage(initialImage(latestPerson.current, result.person, true));
      setStale(false);
      setApplyFailure(null);
      setStep("compare");
    } catch (error) {
      if (isAborted(error) || signal.aborted) return;
      if (candidate === undefined && isMissingId(error)) {
        setByName(true);
        setMissingId(true);
        setStep("find");
        return;
      }
      failed(error, "fetch");
    } finally {
      if (!signal.aborted) setRunning(null);
    }
  };

  const apply = async (current: PersonFetched) => {
    const signal = begin();
    setApplying(true);
    setApplyFailure(null);
    try {
      const applied = await applyPersonPluginFetch(
        person.id,
        current.result.fetchId,
        selection,
        image,
        signal,
      );
      onApplied(applied.person);
      const lines = skippedPersonLines(applied.skipped);
      if (lines.length === 0) {
        toast(t.credits.personPage.pluginFetch.applied(applied.person.name));
        onClose();
        return;
      }
      setSkipped(lines);
      setStep("result");
    } catch (error) {
      if (isAborted(error) || signal.aborted) return;
      if (isCreditGone(error)) gone();
      else if (error instanceof RequestFailed && error.code === "work_info_fetch_stale")
        setStale(true);
      else setApplyFailure(error);
    } finally {
      if (!signal.aborted) setApplying(false);
    }
  };

  const submit = () => {
    if (step === "result") return onClose();
    if (step === "compare") {
      if (
        fetched !== null &&
        !applying &&
        !stale &&
        running === null &&
        (selection.length > 0 || image !== null)
      )
        void apply(fetched);
      return;
    }
    if (plugin === undefined || running !== null) return;
    if (!byName) void fetchPerson(plugin);
    else if (query.trim() !== "") void search(plugin);
  };

  return {
    plugin,
    byName,
    missingId,
    query,
    candidates,
    running,
    failure,
    step,
    fetched,
    selection,
    image,
    chooseImage: setImage,
    stale,
    applyFailure,
    applying,
    skipped,
    submit,
    choosePlugin: (id: number) => {
      setPluginId(id);
      setByName(false);
      setMissingId(false);
      setCandidates(null);
      setFailure(null);
    },
    searchByName: () => {
      if (running !== null || applying) return;
      setByName(true);
      setFailure(null);
      setStep("find");
    },
    changeQuery: setQuery,
    choose: (candidate: CandidateId) => {
      if (plugin !== undefined && running === null) void fetchPerson(plugin, candidate);
    },
    toggle: (item: PersonPluginItem, on: boolean) =>
      setSelection((current) =>
        on ? [...new Set([...current, item])] : current.filter((entry) => entry !== item),
      ),
    back: () => setStep("find"),
    fetchAgain: () => {
      if (fetched === null || running !== null) return;
      void fetchPerson(fetched.plugin, fetched.candidate);
    },
    cancel: () => controller.current?.abort(),
  };
}

export type PersonFetchFlow = ReturnType<typeof usePersonFetchFlow>;

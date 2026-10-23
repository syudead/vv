import { useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "../../../api/client";
import { isCreditGone, type Person } from "../../../api/credits";
import {
  applyPersonPluginFetch,
  fetchPersonWithPlugin,
  type PersonFetchKey,
  type PersonPluginCandidate,
  type PersonPluginFetch,
  searchPersonWithPlugin,
} from "../../../api/personPluginFetches";
import type { WorkInfoPlugin } from "../../../api/workInfoPlugins";
import { t } from "../../../i18n";
import { hostOf, type SkippedLine } from "../../../player/workInfoFetch/fetchRules";
import type { RunFailure } from "../../../player/workInfoFetch/FindStep";
import { useToast } from "../../../ui/Toast";
import {
  initialSelection,
  type PersonSelection,
  selectionBody,
  selectionEmpty,
  skippedPersonLines,
} from "./personCompareRows";

const PLUGIN_GONE = new Set(["plugin_not_approved", "not_found"]);

export type Step = "find" | "compare" | "result";

const EMPTY_SELECTION: PersonSelection = {
  items: [],
  image: null,
  aliases: [],
  externalIds: [],
  links: [],
};

export interface CandidateId {
  source: string;
  id: string;
}

export interface PersonFetched {
  result: PersonPluginFetch;
  plugin: WorkInfoPlugin;
  /** What the dialog sent: a chosen candidate or a picked or typed link; absent when the server picked the key. */
  key?: PersonFetchKey;
}

export interface PersonRunning {
  operation: "search" | "fetch";
  /** The chosen candidate's key, or the picked link, while its fetch runs. */
  key?: string;
}

/** The reason a typed link was refused, shown under the field until the next edit. */
export interface LinkRefusal {
  plugin: WorkInfoPlugin;
  host: string;
}

export function candidateKey(candidate: CandidateId): string {
  return `${candidate.source}\u0000${candidate.id}`;
}

/**
 * The link to fetch by when the Name field holds an http or https URL, else undefined. The browser's serialization
 * is sent because the server's URL parser rejects forms the browser repairs, such as `https:example.com`.
 */
export function typedLink(value: string): string | undefined {
  try {
    const url = new URL(value.trim());
    return url.protocol === "http:" || url.protocol === "https:" ? url.href : undefined;
  } catch {
    return undefined;
  }
}

/** Whether the Name field holds a link to fetch by instead of words to search. */
export function isLink(value: string): boolean {
  return typedLink(value) !== undefined;
}

function failedWith(error: unknown, code: string): error is RequestFailed {
  return error instanceof RequestFailed && error.code === code;
}

function runningKey(
  key: PersonFetchKey | undefined,
  byName: boolean,
): string | undefined {
  if (key === undefined) return undefined;
  if ("link" in key) return byName ? undefined : key.link;
  return candidateKey(key);
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
  // The person's links the plugin handles, after the server answers person_link_ambiguous.
  const [links, setLinks] = useState<readonly string[] | null>(null);
  const [linkRefusal, setLinkRefusal] = useState<LinkRefusal | null>(null);
  const resetPlugin = (id: number | undefined) => {
    setPluginId(id);
    setByName(false);
    setMissingId(false);
    setCandidates(null);
    setLinks(null);
    setLinkRefusal(null);
  };
  if (plugin?.id !== pluginId) resetPlugin(plugin?.id);
  const [running, setRunning] = useState<PersonRunning | null>(null);
  const [failure, setFailure] = useState<RunFailure | null>(null);
  const [step, setStep] = useState<Step>("find");
  const [fetched, setFetched] = useState<PersonFetched | null>(null);
  const [selection, setSelection] = useState<PersonSelection>(EMPTY_SELECTION);
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

  const search = async (target: WorkInfoPlugin, words = query) => {
    const signal = begin();
    setRunning({ operation: "search" });
    setFailure(null);
    try {
      setCandidates(await searchPersonWithPlugin(target.id, words.trim(), signal));
    } catch (error) {
      if (isAborted(error) || signal.aborted) return;
      failed(error, "search");
    } finally {
      if (!signal.aborted) setRunning(null);
    }
  };

  /** Answers the server gives before running: no key, several links, or a link the plugin does not handle. */
  const refused = (error: unknown, target: WorkInfoPlugin, key?: PersonFetchKey) => {
    if (key === undefined && failedWith(error, "person_external_id_missing")) {
      setByName(true);
      setMissingId(true);
    } else if (key === undefined && failedWith(error, "person_link_ambiguous")) {
      setLinks(error.links ?? []);
    } else if (
      key !== undefined &&
      "link" in key &&
      failedWith(error, "person_link_unsupported")
    ) {
      setLinkRefusal({ plugin: target, host: hostOf(key.link) });
    } else return false;
    setStep("find");
    return true;
  };

  const fetchPerson = async (
    target: WorkInfoPlugin,
    key: PersonFetchKey | undefined,
    fromName: boolean,
  ) => {
    const signal = begin();
    setRunning({ operation: "fetch", key: runningKey(key, fromName) });
    setFailure(null);
    setLinkRefusal(null);
    try {
      const result = await fetchPersonWithPlugin(person.id, target.id, key, signal);
      if (signal.aborted) return;
      setFetched({ result, plugin: target, key });
      setSelection(initialSelection(latestPerson.current, result.person, true));
      setStale(false);
      setApplyFailure(null);
      setStep("compare");
    } catch (error) {
      if (isAborted(error) || signal.aborted) return;
      if (!refused(error, target, key)) failed(error, "fetch");
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
        selectionBody(selection),
        selection.image,
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
        !selectionEmpty(selection)
      )
        void apply(fetched);
      return;
    }
    if (plugin === undefined || running !== null) return;
    // The link rows are the action while the choice is shown.
    const link = typedLink(query);
    if (!byName) {
      if (links === null) void fetchPerson(plugin, undefined, false);
    } else if (link !== undefined) void fetchPerson(plugin, { link }, true);
    else if (query.trim() !== "") void search(plugin);
  };

  return {
    plugin,
    byName,
    missingId,
    query,
    candidates,
    links,
    linkRefusal,
    running,
    failure,
    step,
    fetched,
    selection,
    changeSelection: setSelection,
    stale,
    applyFailure,
    applying,
    skipped,
    submit,
    choosePlugin: (id: number) => {
      resetPlugin(id);
      setFailure(null);
    },
    searchByName: () => {
      if (running !== null || applying) return;
      setByName(true);
      setFailure(null);
      setStep("find");
    },
    changeQuery: (value: string) => {
      setQuery(value);
      setLinkRefusal(null);
    },
    /** Searches for one of the person's names at once. */
    searchFor: (name: string) => {
      if (plugin === undefined || running !== null) return;
      setQuery(name);
      setLinkRefusal(null);
      void search(plugin, name);
    },
    choose: (key: PersonFetchKey) => {
      if (plugin !== undefined && running === null) void fetchPerson(plugin, key, false);
    },
    back: () => setStep("find"),
    /** Resends what the result came from, so a picked or typed link is not asked for again. */
    fetchAgain: () => {
      if (fetched === null || running !== null) return;
      void fetchPerson(fetched.plugin, fetched.key, byName);
    },
    cancel: () => controller.current?.abort(),
  };
}

export type PersonFetchFlow = ReturnType<typeof usePersonFetchFlow>;

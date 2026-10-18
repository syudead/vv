import { useEffect, useEffectEvent, useId, useMemo, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  listTagPage,
  mergeTag,
  tagImpact,
  type Tag,
  type TagImpactResponse,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Badge } from "../ui/shadcn/badge";
import { Field, FieldTitle } from "../ui/shadcn/field";
import { Spinner } from "../ui/shadcn/spinner";
import TagCommand, { type TagChoice } from "../ui/TagCommand";
import TentativeMark from "../ui/TentativeMark";
import { DialogError } from "./DialogError";
import { MergeSummary, MergeTargetField } from "./MergeParts";

/** Search all server tags with exact matches first. Selection merges exclude the chosen target from sources and require refreshed impact counts. */
export default function MergeTagDialog({
  sources,
  fromSelection = false,
  onClose,
  onMerged,
  onStale,
}: {
  /** Freeze nonempty sources when opening. */
  sources: readonly Tag[];
  fromSelection?: boolean;
  onClose: () => void;
  /** Pass before/after target state and missing source IDs so callers can update filtered counts, including unloaded sources. */
  onMerged: (
    merged: Tag,
    sourceIds: readonly number[],
    notFoundIds: readonly number[],
    target: Tag,
  ) => void;
  onStale: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  const mergeButton = useRef<HTMLButtonElement>(null);
  const sourcesHeadingId = useId();
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Tag | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [impact, setImpact] = useState<TagImpactResponse | null>(null);
  const [countError, setCountError] = useState<UiText | null>(null);
  const [candidates, setCandidates] = useState<readonly Tag[]>([]);
  const [exactCandidate, setExactCandidate] = useState<Tag | null>(null);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<UiText | null>(null);
  // Recount on retry or reselection even if target ID and effective sources are unchanged.
  const [attempt, setAttempt] = useState(0);
  // Focus Merge once per target choice, only after impact counts enable it.
  const justSelectedRef = useRef(false);

  const excluded = useMemo(
    () => (fromSelection ? new Set<number>() : new Set(sources.map((item) => item.id))),
    [fromSelection, sources],
  );
  const {
    tags: targetTags,
    options,
    exactOption,
  } = buildTargetOptions(candidates, exactCandidate, excluded, value);
  // Overfetch row-dialog candidates to keep eight options after excluding sources.
  const requestLimit = Math.min(targetCandidateLimit + excluded.size, tagPageMaxLimit);

  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    setSearchError(null);
    listTagPage({ q: value.trim(), limit: requestLimit }, controller.signal)
      .then((page) => {
        if (controller.signal.aborted) return;
        setCandidates(page.items);
        setExactCandidate(page.exact ?? null);
        setSearching(false);
      })
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        setSearchError(errorText(failure));
        setSearching(false);
      });
    return () => controller.abort();
  }, [value, requestLimit]);

  const targetId = target?.id ?? null;
  const effective = useMemo(
    () => sources.filter((item) => item.id !== targetId),
    [sources, targetId],
  );
  const kept = targetId !== null && effective.length < sources.length;

  useEffect(() => {
    setImpact(null);
    setCountError(null);
    if (!fromSelection || targetId === null || effective.length === 0) return;
    const controller = new AbortController();
    tagImpact(
      "merge",
      effective.map((item) => item.id),
      controller.signal,
    )
      .then((result) => {
        if (!controller.signal.aborted) setImpact(result);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setCountError(errorText(failure));
      });
    return () => controller.abort();
  }, [fromSelection, targetId, effective, attempt]);

  const counted = !fromSelection || impact !== null;
  const canSubmit = target !== null && effective.length > 0 && counted && !pending;

  useEffect(() => {
    if (justSelectedRef.current && target !== null && canSubmit) {
      justSelectedRef.current = false;
      mergeButton.current?.focus();
    }
  }, [target, canSubmit]);

  // Restore actionable focus after errors; disabled Merge may have dropped focus to body.
  // （N4）。
  // React only to errors, not target reselection.
  const restoreFocus = useEffectEvent(() => {
    (target !== null ? mergeButton : cancel).current?.focus();
  });
  useEffect(() => {
    if (error !== null) restoreFocus();
  }, [error]);

  /** Block every close path while merging so pending responses cannot update a removed dialog. */
  function handleClose() {
    if (pending) return;
    onClose();
  }

  function selectTarget(option: TagChoice) {
    const found = targetTags.find((item) => String(item.id) === option.id);
    if (found === undefined) return;
    justSelectedRef.current = true;
    setTarget(found);
    setAttempt((current) => current + 1);
    setValue(found.name);
    setError(null);
  }

  async function submit() {
    if (target === null || !canSubmit) return;
    const sourceIds = effective.map((item) => item.id);
    setError(null);
    setPending(true);
    try {
      const { tag: merged, notFoundIds } = await mergeTag(target.id, sourceIds);
      // All-missing sources performed no merge; close and refresh as for a missing target.
      if (notFoundIds.length >= sourceIds.length) {
        onStale();
        return;
      }
      onMerged(merged, sourceIds, notFoundIds, target);
    } catch (failure) {
      if (failure instanceof RequestFailed && failure.code === "tag_not_found") {
        onStale();
        return;
      }
      setError(errorText(failure));
      setPending(false);
    }
  }

  const single = sources.length === 1 ? sources[0] : undefined;
  const title =
    single !== undefined
      ? t.tags.mergeDialog.title(single.name)
      : t.tags.mergeDialog.titleMany(sources.length);

  let message: UiText | null = null;
  if (target !== null && effective.length > 0 && counted) {
    const only = effective.length === 1 ? effective[0] : undefined;
    const videoCount = impact?.videoCount ?? only?.videoCount ?? 0;
    message =
      only !== undefined
        ? t.tags.mergeDialog.warning(only.name, videoCount, target.name)
        : t.tags.mergeDialog.warningMany(effective.length, videoCount, target.name);
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
      title={title}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.tags.mergeDialog.submitting : t.tags.mergeDialog.submit}
        </>
      }
      pending={pending}
      submitDisabled={!canSubmit}
      submitRef={mergeButton}
      cancelRef={cancel}
      initialFocus={cancel}
    >
      {sources.length > 1 && (
        <Field>
          <FieldTitle id={sourcesHeadingId}>{t.tags.mergeDialog.sources}</FieldTitle>
          {/* Scroll only the sources so target selection and confirmation remain visible. */}
          <ul
            aria-labelledby={sourcesHeadingId}
            className="flex max-h-16 flex-wrap gap-1 overflow-y-auto"
          >
            {sources.map((item) => (
              <li key={item.id} className="max-w-full min-w-0">
                <Badge variant="secondary" title={item.name} className="max-w-full">
                  <span className="min-w-0 truncate">{item.name}</span>
                  {item.tentative && (
                    <>
                      <TentativeMark />
                      <span className="sr-only">{t.tags.tentative}</span>
                    </>
                  )}
                  {item.id === targetId && (
                    <span className="font-normal text-muted-foreground">
                      {t.tags.mergeDialog.kept}
                    </span>
                  )}
                </Badge>
              </li>
            ))}
          </ul>
        </Field>
      )}
      {}
      <MergeTargetField
        Command={TagCommand}
        label={t.tags.mergeDialog.target}
        value={value}
        onValueChange={(next) => {
          setValue(next);
          setTarget(null);
          setError(null);
        }}
        choices={options}
        exactChoice={exactOption}
        onSelect={selectTarget}
        chosenId={target === null ? undefined : String(target.id)}
        searching={searching}
        searchError={
          searchError === null ? null : t.tags.mergeDialog.searchFailed(searchError)
        }
        emptyText={t.tags.mergeDialog.noCandidates}
      />
      {target !== null && effective.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t.tags.mergeDialog.onlyTarget(target.name)}
        </p>
      )}
      {target !== null && kept && effective.length > 0 && (
        <p className="text-sm text-muted-foreground">
          {t.tags.mergeDialog.keptNote(target.name, effective.length)}
        </p>
      )}
      {fromSelection &&
        target !== null &&
        effective.length > 0 &&
        impact === null &&
        countError === null && (
          <p
            aria-busy="true"
            className="flex items-center gap-2 text-sm text-muted-foreground"
          >
            <Spinner aria-hidden="true" />
            {t.tags.bulkDialog.counting}
          </p>
        )}
      {countError !== null && (
        <DialogError
          message={t.tags.bulkDialog.countFailed(countError)}
          onRetry={() => setAttempt((current) => current + 1)}
        />
      )}
      {message !== null && <p className="text-sm text-muted-foreground">{message}</p>}
      {target !== null && effective.length > 0 && (
        <MergeSummary
          source={
            effective.length === 1
              ? effective[0]!.name
              : t.tags.mergeDialog.summarySources(effective.length)
          }
          target={target.name}
        />
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}

const targetCandidateLimit = 8;

const tagPageMaxLimit = 200;

/** Prioritize exact only when it still matches current input; otherwise preserve server order and show matched-synonym hints. */
function buildTargetOptions(
  items: readonly Tag[],
  exact: Tag | null,
  excluded: ReadonlySet<number>,
  input: string,
): { tags: Tag[]; options: TagChoice[]; exactOption: TagChoice | null } {
  const trimmed = input.trim();
  const query = foldForMatch(trimmed);
  const spelled = (tag: Tag) =>
    trimmed !== "" && (tag.name === trimmed || tag.synonyms.includes(trimmed));
  const allowed = items.filter((tag) => !excluded.has(tag.id));

  const exactTag =
    exact !== null && !excluded.has(exact.id) && spelled(exact)
      ? exact
      : allowed.find(spelled);
  const candidates = (
    exactTag === undefined
      ? allowed
      : [exactTag, ...allowed.filter((tag) => tag.id !== exactTag.id)]
  ).slice(0, targetCandidateLimit);

  const options: TagChoice[] = candidates.map((tag) => {
    const nameMatch = query === "" || foldForMatch(tag.name).includes(query);
    const synonymHit = nameMatch
      ? undefined
      : tag.synonyms.find((synonym) => foldForMatch(synonym).includes(query));
    return {
      id: String(tag.id),
      label: tag.name,
      hint:
        synonymHit !== undefined ? t.tags.mergeDialog.synonymHint(synonymHit) : undefined,
      meta: t.tags.mergeDialog.videoCount(tag.videoCount),
    };
  });

  const exactOption: TagChoice | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: t.tags.mergeDialog.videoCount(exactTag.videoCount),
        };

  return { tags: candidates, options, exactOption };
}

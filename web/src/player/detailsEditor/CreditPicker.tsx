import { Plus } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { isAborted } from "../../api/client";
import {
  type Credit,
  type CreditKind,
  listCredits,
  listPeople,
  type Person,
  personFaceSrc,
} from "../../api/credits";
import { errorText, t, type UiText } from "../../i18n";
import { foldForMatch } from "../../lib/foldForMatch";
import CreditCommand, { type CreditChoice } from "../../ui/CreditCommand";
import { PersonFace } from "../../ui/PersonFace";
import { type Chip, isNewChip, newRowKey } from "./draft";

const candidateLimit = 8;

interface Candidates {
  /** Trimmed spelling associated with this response. */
  q: string;
  items: Credit[];
  more: boolean;
}

export interface UnsavedName {
  newKey: string;
  name: string;
}

function matchesExactly(credit: Credit, folded: string): boolean {
  return (
    foldForMatch(credit.name) === folded ||
    credit.aliases.some((alias) => foldForMatch(alias.name) === folded)
  );
}

function aliasHint(credit: Credit, folded: string): string | undefined {
  if (folded === "" || foldForMatch(credit.name).includes(folded)) return undefined;
  return credit.aliases.find((alias) => foldForMatch(alias.name).includes(folded))?.name;
}

/** Show eight candidates, but resolve exact matches without a limit before creating on Enter. Keep prior rows while loading and abort obsolete requests. */
export default function CreditPicker({
  kind,
  offerUnnamed = false,
  label,
  chips,
  unsaved,
  disabled,
  onAdd,
}: {
  kind: CreditKind;
  /** Lists provisional people in an Unnamed people group under the name results. */
  offerUnnamed?: boolean;
  label: UiText;
  chips: readonly Chip[];
  unsaved: readonly UnsavedName[];
  disabled: boolean;
  onAdd: (chip: Chip) => void;
}) {
  const failureId = useId();
  const [value, setValue] = useState("");
  const [candidates, setCandidates] = useState<Candidates | null>(null);
  const [searching, setSearching] = useState(false);
  const [resolving, setResolving] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const request = useRef<AbortController | null>(null);
  const trimmed = value.trim();
  const folded = foldForMatch(trimmed);

  useEffect(() => {
    // Typing aborts Enter resolution; its aborted finally cannot reset resolving, so reset it here.
    request.current?.abort();
    setResolving(false);
    if (trimmed === "") {
      setSearching(false);
      return;
    }
    const controller = new AbortController();
    request.current = controller;
    setSearching(true);
    listCredits(kind, { q: trimmed, limit: candidateLimit }, controller.signal).then(
      (page) => {
        if (controller.signal.aborted) return;
        setCandidates({
          q: trimmed,
          items: page.items,
          more: page.nextCursor !== undefined,
        });
        setFailure(null);
        setSearching(false);
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        setFailure(searchFailed(kind, error));
        setSearching(false);
      },
    );
    return () => controller.abort();
  }, [kind, trimmed]);

  const chosenIds = new Set(chips.flatMap((chip) => (isNewChip(chip) ? [] : [chip.id])));
  const unnamed = useUnnamedPeople(offerUnnamed, chips.length, setFailure);
  const unnamedShown = unnamed
    .filter((person) => !chosenIds.has(person.id))
    .slice(0, candidateLimit);
  const chosenNew = new Set(
    chips.flatMap((chip) => (isNewChip(chip) ? [chip.newKey] : [])),
  );
  const available = (candidates?.items ?? []).filter(
    (credit) => !chosenIds.has(credit.id),
  );
  const unsavedMatches =
    folded === ""
      ? []
      : unsaved.filter(
          (entry) =>
            !chosenNew.has(entry.newKey) && foldForMatch(entry.name).includes(folded),
        );
  const exact = available.filter((credit) => matchesExactly(credit, folded));
  const rest = available.filter((credit) => !matchesExactly(credit, folded));

  const choices: CreditChoice[] = [
    ...unsavedMatches.map((entry) => ({
      id: `new:${entry.newKey}`,
      label: entry.name,
      meta: t.player.detailsEditor.newMark,
      ariaLabel: t.player.detailsEditor.newChip(entry.name),
    })),
    ...[...exact, ...rest].map((credit) => {
      const alias = aliasHint(credit, folded);
      return {
        id: String(credit.id),
        label: credit.name,
        hint: alias === undefined ? undefined : t.player.detailsEditor.alias(alias),
        meta: t.player.detailsEditor.videoCount(credit.videoCount),
      };
    }),
  ];

  const unsavedSame = unsaved.some((entry) => entry.name === trimmed);
  const createLabel =
    trimmed === "" || unsavedSame ? null : (
      <span className="flex min-w-0 items-center gap-2">
        <Plus className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="truncate">{t.player.detailsEditor.create(trimmed)}</span>
      </span>
    );

  function add(chip: Chip) {
    setValue("");
    setFailure(null);
    onAdd(chip);
  }

  function pickCredit(credit: Credit) {
    if (chosenIds.has(credit.id)) {
      setValue("");
      return;
    }
    add({ key: newRowKey("c"), id: credit.id, name: credit.name });
  }

  function pick(choice: CreditChoice) {
    if (choice.id.startsWith(unnamedPrefix)) {
      const id = Number(choice.id.slice(unnamedPrefix.length));
      if (chosenIds.has(id)) return;
      // A typed name becomes this person's name on Save.
      add(
        trimmed === ""
          ? { key: newRowKey("c"), id, name: "", provisional: true }
          : { key: newRowKey("c"), id, name: trimmed, naming: true },
      );
      return;
    }
    if (choice.id.startsWith("new:")) {
      add({ key: newRowKey("c"), newKey: choice.id.slice(4), name: choice.label });
      return;
    }
    const credit = available.find((item) => String(item.id) === choice.id);
    if (credit !== undefined) pickCredit(credit);
  }

  function create(spelling: string) {
    add({ key: newRowKey("c"), newKey: newRowKey("n"), name: spelling });
  }

  function commitSpelling(spelling: string) {
    const spellingFolded = foldForMatch(spelling);
    const local = unsaved.find((entry) => foldForMatch(entry.name) === spellingFolded);
    if (local !== undefined) {
      if (!chosenNew.has(local.newKey)) {
        add({ key: newRowKey("c"), newKey: local.newKey, name: local.name });
      } else setValue("");
      return;
    }
    const fresh = candidates !== null && candidates.q === spelling && !searching;
    if (fresh) {
      const match = candidates.items.find((credit) =>
        matchesExactly(credit, spellingFolded),
      );
      if (match !== undefined) {
        pickCredit(match);
        return;
      }
      if (!candidates.more) {
        create(spelling);
        return;
      }
    }
    // An exact match may lie outside the first eight results; search without a limit.
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setSearching(false);
    setResolving(true);
    listCredits(kind, { q: spelling }, controller.signal)
      .then(
        (page) => {
          if (controller.signal.aborted) return;
          const match = page.items.find((credit) =>
            matchesExactly(credit, spellingFolded),
          );
          if (match !== undefined) pickCredit(match);
          else create(spelling);
        },
        (error: unknown) => {
          if (controller.signal.aborted || isAborted(error)) return;
          setFailure(searchFailed(kind, error));
        },
      )
      .finally(() => {
        if (request.current === controller) setResolving(false);
      });
  }

  return (
    <div className="flex max-w-full min-w-0 flex-col">
      <CreditCommand
        layout="dropdown"
        label={label}
        value={value}
        onValueChange={setValue}
        choices={choices}
        group={
          offerUnnamed
            ? {
                heading: t.player.detailsEditor.unnamedPeople,
                choices: unnamedShown.map((person) => unnamedChoice(person, trimmed)),
              }
            : undefined
        }
        exactChoice={null}
        onSelect={pick}
        createLabel={createLabel}
        onCreate={create}
        onCommitSpelling={commitSpelling}
        icon={
          <Plus className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />
        }
        busy={resolving}
        searching={searching}
        locked={disabled}
        describedBy={failure === null ? undefined : failureId}
        onEscape={() => setValue("")}
      />
      {failure !== null && (
        <p id={failureId} className="mt-1 text-xs text-destructive">
          {failure}
        </p>
      )}
    </div>
  );
}

const unnamedPrefix = "unnamed:";

function unnamedChoice(person: Person, typed: string): CreditChoice {
  const e = t.player.detailsEditor;
  const label = t.credits.unnamedPerson;
  const hint = typed === "" ? null : e.addAs(typed);
  return {
    id: `${unnamedPrefix}${String(person.id)}`,
    label,
    quiet: true,
    leading: <PersonFace shape="chip" name={label} src={personFaceSrc(person)} />,
    hint: hint ?? undefined,
    meta: e.videoCount(person.videoCount),
    ariaLabel: e.unnamedRow(label, person.videoCount, hint),
  };
}

/**
 * Provisional people for the picker, read again when the chips change so the shown rows can
 * still fill the candidate limit after the chosen ones are left out.
 */
function useUnnamedPeople(
  enabled: boolean,
  chipCount: number,
  onFailure: (failure: UiText) => void,
): Person[] {
  const [people, setPeople] = useState<Person[]>([]);
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    listPeople(
      { provisional: true, limit: candidateLimit + chipCount },
      controller.signal,
    ).then(
      (page) => {
        if (!controller.signal.aborted) setPeople(page.items);
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        onFailure(searchFailed("person", error));
      },
    );
    return () => controller.abort();
  }, [enabled, chipCount, onFailure]);
  return people;
}

function searchFailed(kind: CreditKind, error: unknown): UiText {
  const reason = errorText(error);
  return kind === "person"
    ? t.player.detailsEditor.searchPeopleFailed(reason)
    : t.player.detailsEditor.searchOrganizationsFailed(reason);
}

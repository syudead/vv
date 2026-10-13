import { Plus } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { isAborted } from "../../api/client";
import { type Credit, type CreditKind, listCredits } from "../../api/credits";
import { errorText, t, type UiText } from "../../i18n";
import { foldForMatch } from "../../lib/foldForMatch";
import CreditCommand, { type CreditChoice } from "../../ui/CreditCommand";
import { type Chip, isNewChip, newRowKey } from "./draft";

/** candidateLimit は、打つたびに読む候補の数である（ui-design.md「Picker」）。 */
const candidateLimit = 8;

/** Candidates は 1 回の候補の読み込みの結果である。 */
interface Candidates {
  /** 読んだときの綴り（前後の空白を除いたもの）。 */
  q: string;
  items: Credit[];
  /** 8 件より多く当たった（`nextCursor` があった）。 */
  more: boolean;
}

/** UnsavedName は下書きにある、まだ作っていない同じ種類の名前である。 */
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

/** aliasHint は、名前でなく別名で当たったときのその別名を返す。 */
function aliasHint(credit: Credit, folded: string): string | undefined {
  if (folded === "" || foldForMatch(credit.name).includes(folded)) return undefined;
  return credit.aliases.find((alias) => foldForMatch(alias.name).includes(folded))?.name;
}

/**
 * CreditPicker は役割 1 つの、人物か団体を選ぶか作る入力である（specs/047-video-work-info/
 * ui-design.md「Picker」）。CreditCommand の `dropdown` で、打つたびに `limit=8` で候補を読む
 * （前の要求は打ち切り、新しい行が届くまで前の行を残す）。この役割のチップにあるものは候補に
 * 出さず、下書きのまだ作っていない同じ種類の名前は `New` の行として出す。作成の行は、
 * 同じ綴りのまだ作っていない名前が無ければ、一致する人物がいても出す。
 *
 * 矢印を押さずに Enter を押したときは、完全一致があればそれを選び、8 件の外にあるかも
 * しれないとき（`nextCursor` があった、または読み終えていない）は `limit` なしで読み直して
 * 探し、どこにも無いときだけ作る。読めなかったら理由を出し、チップは足さない。
 */
export default function CreditPicker({
  kind,
  label,
  chips,
  unsaved,
  disabled,
  onAdd,
}: {
  kind: CreditKind;
  /** 入力の名前と placeholder（「Add a performer」）。 */
  label: UiText;
  /** この役割の今のチップ。 */
  chips: readonly Chip[];
  /** 下書き全体の、まだ作っていない同じ種類の名前。 */
  unsaved: readonly UnsavedName[];
  /** 保存の送信中。何も受け付けない。 */
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
    request.current?.abort();
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
    // 8 件の外に完全一致があるかもしれない。limit なしで読み直して探す。
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
        // 一覧が閉じているときの Esc は、入力を空にする（タグの入力と同じ）。
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

function searchFailed(kind: CreditKind, error: unknown): UiText {
  const reason = errorText(error);
  return kind === "person"
    ? t.player.detailsEditor.searchPeopleFailed(reason)
    : t.player.detailsEditor.searchOrganizationsFailed(reason);
}

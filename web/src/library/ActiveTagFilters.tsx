import { Building2, Shapes, Tag as TagIcon, User, X } from "lucide-react";
import { type ReactNode, type RefObject, useEffect, useRef } from "react";

import type { CreditKind } from "../api/credits";
import { compareNatural } from "../api/tagOrder";
import { t } from "../i18n";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Toggle } from "../ui/shadcn/toggle";
import { type CreditFilter, creditKinds } from "./creditCriteria";
import { useCreditName, useTagNames } from "./filterNames";

export interface ActiveTagFiltersProps {
  /** 絞り込み中のタグの id（並びは問わない。表示は名前の自然順にそろえる）。 */
  tagIds: readonly number[];
  onRemove: (id: number) => void;
  /** 絞り込み中の人物・団体（タグのチップの後に、人物、団体の順で 1 つずつ出す）。 */
  credits?: CreditFilter;
  onRemoveCredit?: (kind: CreditKind) => void;
  /** 名前を読んだら `404` だった（もう無い）人物・団体。呼び手が URL から外して知らせる。 */
  onCreditGone?: (kind: CreditKind) => void;
  /**
   * 絞り込み中のタグの種類の名前（`credits.tagKind` があるときだけ使う）。種類の一覧が
   * 届く前は undefined で、名前の代わりに Skeleton を出す。
   */
  tagKindName?: string;
  onRemoveTagKind?: () => void;
  /** 最後の1つを外したときのフォーカスの移し先（検索欄）。 */
  searchFieldRef: RefObject<HTMLInputElement | null>;
}

/** ChipKey はチップの鍵である（タグは `tag:<id>`、人物・団体は種類）。 */
type ChipKey = string;

/** tagKindKey はタグの種類のチップの鍵である。 */
const tagKindKey: ChipKey = "tagKind";

function tagKey(id: number): ChipKey {
  return `tag:${String(id)}`;
}

interface ChipProps {
  icon: ReactNode;
  name: string | undefined;
  /** 名前が分かる前の読み上げ名。 */
  unnamedLabel: string;
  /** 名前が分かったあとの読み上げ名。省けば「〈名〉の絞り込みを外す」。 */
  namedLabel?: (name: string) => string;
  /** チップに出す文字。省けば名前そのもの。 */
  text?: (name: string) => string;
  onPress: () => void;
  buttonRef: (node: HTMLButtonElement | null) => void;
}

/**
 * FilterChip は効いている絞り込み 1 つのチップである。効いている絞り込みは押された
 * Toggle で、押し戻すと外れる（web/registry/rules/components.md「Toggle and ToggleGroup」）。
 */
function FilterChip({
  icon,
  name,
  unnamedLabel,
  namedLabel = t.library.activeTags.removeNamed,
  text = (shown) => shown,
  onPress,
  buttonRef,
}: ChipProps) {
  // 名前をまだ読んでいない間も、読み上げる名前が無くならないようにする（N2）。
  // 名前が分かれば「〈名〉の絞り込みを外す」に差し替わる。
  const label = name === undefined ? unnamedLabel : namedLabel(name);
  return (
    <li>
      <Toggle
        ref={buttonRef}
        variant="outline"
        size="sm"
        pressed
        onPressedChange={onPress}
        aria-label={label}
        title={label}
      >
        {icon}
        {name === undefined ? (
          <Skeleton className="h-3 w-12" />
        ) : (
          <span title={text(name)} className="max-w-chip-label min-w-0 truncate">
            {text(name)}
          </span>
        )}
        <X aria-hidden="true" />
      </Toggle>
    </li>
  );
}

function CreditChip({
  kind,
  id,
  onGone,
  ...rest
}: Omit<ChipProps, "icon" | "name" | "unnamedLabel"> & {
  kind: CreditKind;
  id: number;
  onGone: (kind: CreditKind) => void;
}) {
  const name = useCreditName(kind, id, onGone);
  const person = kind === "person";
  return (
    <FilterChip
      {...rest}
      icon={person ? <User aria-hidden="true" /> : <Building2 aria-hidden="true" />}
      name={name}
      unnamedLabel={
        person
          ? t.library.activeTags.removePerson
          : t.library.activeTags.removeOrganization
      }
    />
  );
}

const noCredits: CreditFilter = {};
const ignoreCredit = () => undefined;

/**
 * ActiveTagFilters はライブラリの本文の先頭、要約行の上に置く「絞り込み中の
 * 条件」の行である。
 * タグのチップの後に人物・団体のチップを 1 つずつ、その後にタグの種類のチップを並べる。タグ・人物・団体・タグの種類の
 * どれかで絞り込んでいるときだけ描画する（呼び出し側が出し分ける）。
 */
export default function ActiveTagFilters({
  tagIds,
  onRemove,
  credits = noCredits,
  onRemoveCredit = ignoreCredit,
  onCreditGone = ignoreCredit,
  tagKindName,
  onRemoveTagKind = ignoreCredit,
  searchFieldRef,
}: ActiveTagFiltersProps) {
  const byId = useTagNames(tagIds);

  const buttonRefs = useRef(new Map<ChipKey, HTMLButtonElement>());
  const pendingFocus = useRef<ChipKey | "search" | null>(null);
  useEffect(() => {
    const pending = pendingFocus.current;
    if (pending === null) return;
    pendingFocus.current = null;
    if (pending === "search") {
      searchFieldRef.current?.focus();
      return;
    }
    buttonRefs.current.get(pending)?.focus();
  }, [tagIds, credits.person, credits.organization, credits.tagKind, searchFieldRef]);

  const orderedTags = [...tagIds].sort((a, b) => {
    const nameA = byId.get(a)?.name;
    const nameB = byId.get(b)?.name;
    if (nameA === undefined || nameB === undefined) return a - b;
    return compareNatural(nameA, nameB) || a - b;
  });
  const activeCredits = creditKinds.flatMap((kind) => {
    const id = credits[kind];
    return id === undefined ? [] : [{ kind, id }];
  });
  const ordered: ChipKey[] = [
    ...orderedTags.map(tagKey),
    ...activeCredits.map(({ kind }) => kind),
    ...(credits.tagKind === undefined ? [] : [tagKindKey]),
  ];

  // 外したチップの次、無ければ前のチップへフォーカスを移す。最後の1つを外すと、
  // この行は呼び出し側で描画されなくなり、フォーカスを移す効果も走らないので、
  // 外す前に検索欄へ移しておく。
  function removeChip(key: ChipKey, remove: () => void) {
    const index = ordered.indexOf(key);
    const next = ordered[index + 1] ?? ordered[index - 1];
    if (next === undefined) {
      pendingFocus.current = null;
      searchFieldRef.current?.focus();
    } else {
      pendingFocus.current = next;
    }
    remove();
  }

  const refFor = (key: ChipKey) => (node: HTMLButtonElement | null) => {
    if (node) buttonRefs.current.set(key, node);
    else buttonRefs.current.delete(key);
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs text-muted-foreground">{t.library.activeTags.label}</span>
      <ul
        aria-label={t.library.activeTags.list}
        className="flex flex-wrap items-center gap-2"
      >
        {orderedTags.map((id) => (
          <FilterChip
            key={tagKey(id)}
            buttonRef={refFor(tagKey(id))}
            icon={<TagIcon aria-hidden="true" />}
            name={byId.get(id)?.name}
            unnamedLabel={t.library.activeTags.remove}
            onPress={() => removeChip(tagKey(id), () => onRemove(id))}
          />
        ))}
        {activeCredits.map(({ kind, id }) => (
          <CreditChip
            // id が変わったら名前を読み直す（鍵に id を入れて作り直す）。
            key={`${kind}:${String(id)}`}
            kind={kind}
            id={id}
            buttonRef={refFor(kind)}
            onGone={onCreditGone}
            onPress={() => removeChip(kind, () => onRemoveCredit(kind))}
          />
        ))}
        {credits.tagKind !== undefined && (
          <FilterChip
            buttonRef={refFor(tagKindKey)}
            icon={<Shapes aria-hidden="true" />}
            name={tagKindName}
            unnamedLabel={t.library.activeTags.removeKind}
            namedLabel={t.library.activeTags.removeKindNamed}
            text={t.library.activeTags.kindChip}
            onPress={() => removeChip(tagKindKey, onRemoveTagKind)}
          />
        )}
      </ul>
    </div>
  );
}

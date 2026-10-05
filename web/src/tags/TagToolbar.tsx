import {
  ArrowDownAZ,
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  CalendarPlus,
  Hash,
  type LucideIcon,
} from "lucide-react";
import type { Ref, RefObject } from "react";

import { t } from "../i18n";
import { Toolbar } from "../ui/patterns/toolbar";
import { FilterCheckbox, FilterPopover } from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import IconToggleGroup from "../videoList/IconToggleGroup";
import {
  CompactSortView,
  SortMenuView,
  type SortOption,
} from "../videoList/SortControls";
import {
  tagSortDirection,
  tagSortKind,
  tagSortKinds,
  withTagSortDirection,
  type TagListSort,
  type TagSortDirection,
  type TagSortKind,
} from "./tagListOrder";

/**
 * tagSortIcons は種類ごとのアイコンである（specs/036-tag-admin-scale/ui-design.md
 * 「Top bar」）。「Date created」はライブラリのファイルの作成日（`FileClock`）と
 * 別のものなので、同じ絵にしない。
 */
const tagSortIcons: Record<TagSortKind, LucideIcon> = {
  name: ArrowDownAZ,
  count: Hash,
  created: CalendarPlus,
};

/** tagSortOptions はメニューに並べる種類で、値は選んだときの既定の並び順である。 */
function tagSortOptions(): SortOption<TagListSort>[] {
  return tagSortKinds.map((option) => ({
    value: option.initial,
    label: t.tags.sort.kinds[option.kind],
    icon: tagSortIcons[option.kind],
  }));
}

/** activeOption は今の並び順の種類の、メニューでの値（種類の既定の並び順）である。 */
function activeOption(sort: TagListSort): TagListSort {
  const kind = tagSortKind(sort);
  return tagSortKinds.find((option) => option.kind === kind)!.initial;
}

export interface TagToolbarProps {
  query: string;
  onQueryCommit: (next: string, mode: HistoryMode) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  searchDisabled: boolean;
  tentativeOnly: boolean;
  onTentativeOnlyChange: (value: boolean) => void;
  unusedOnly: boolean;
  onUnusedOnlyChange: (value: boolean) => void;
  /** 「Tentative only」「Unused only」を両方外す（吹き出しの「Clear filters」）。 */
  onClearFilters: () => void;
  filterDisabled: boolean;
  filterRef: Ref<HTMLButtonElement>;
  sort: TagListSort;
  onSortChange: (sort: TagListSort) => void;
  sortDisabled: boolean;
}

/**
 * TagToolbar はタグ管理画面の操作を共通トップバーの中央に置く（デザインシステムの
 * `Toolbar` の placement="topBar"。TagsPage が `TopBarPortal` で入れる。
 * web/registry/rules/patterns.md の Sections）。並びはライブラリの `LibraryToolbar` と同じで、
 * 検索欄 → 絞り込み → 並び順（と向き）。検索欄・絞り込み・並び順の部品はライブラリと共有する
 * `web/src/videoList/` のもので、並び順は `md` から並べ、それより狭いとアイコンだけの
 * 「Sort」のまとめ（ライブラリの `CompactSortControls` と同じ形）に入れる
 * （specs/036-tag-admin-scale/ui-design.md「Top bar」）。
 */
export default function TagToolbar({
  query,
  onQueryCommit,
  searchRef,
  searchDisabled,
  tentativeOnly,
  onTentativeOnlyChange,
  unusedOnly,
  onUnusedOnlyChange,
  onClearFilters,
  filterDisabled,
  filterRef,
  sort,
  onSortChange,
  sortDisabled,
}: TagToolbarProps) {
  const kind = tagSortKind(sort);
  const direction = tagSortDirection(sort);
  const options = tagSortOptions();
  const filterCount = (tentativeOnly ? 1 : 0) + (unusedOnly ? 1 : 0);

  return (
    <Toolbar
      placement="topBar"
      search={
        <SearchBox
          query={query}
          onCommit={onQueryCommit}
          inputRef={searchRef}
          label={t.tags.search.label}
          placeholder={t.tags.search.placeholder}
          syntaxHelp={false}
          // 打鍵ごとに引き直す（前の要求は打ち切る。specs/036-tag-admin-scale/research.md R-1）。
          debounceMs={0}
          disabled={searchDisabled}
        />
      }
      view={[
        {
          id: "sort",
          label: t.tags.sort.heading,
          inlineFrom: "md",
          compactLabelled: true,
          compact: (
            <CompactSortView
              name="tag-compact-sort"
              heading={t.tags.sort.heading}
              value={activeOption(sort)}
              options={options}
              onValueChange={(next) => {
                if (tagSortKind(next) !== kind) onSortChange(next);
              }}
              disabled={sortDisabled}
            >
              {direction !== undefined && (
                <IconToggleGroup<TagSortDirection>
                  label={t.tags.sort.direction}
                  value={direction}
                  onValueChange={(next) => onSortChange(withTagSortDirection(sort, next))}
                  options={[
                    {
                      value: "desc",
                      label:
                        kind === "count"
                          ? t.tags.sort.segments.countDesc
                          : t.tags.sort.segments.createdDesc,
                      icon: <ArrowDownWideNarrow aria-hidden="true" />,
                    },
                    {
                      value: "asc",
                      label:
                        kind === "count"
                          ? t.tags.sort.segments.countAsc
                          : t.tags.sort.segments.createdAsc,
                      icon: <ArrowUpNarrowWide aria-hidden="true" />,
                    },
                  ]}
                />
              )}
            </CompactSortView>
          ),
          control: (
            <SortMenuView
              label={t.tags.sort.kinds[kind]}
              currentLabel={t.tags.sort.current(t.tags.sort.kinds[kind])}
              heading={t.tags.sort.heading}
              value={activeOption(sort)}
              options={options}
              onValueChange={(next) => {
                if (tagSortKind(next) !== kind) onSortChange(next);
              }}
              // 「Name」に向きは無いので、そのときは向きのボタンを出さない。
              toggle={
                direction === undefined || sort === "name"
                  ? undefined
                  : {
                      label: t.tags.sort.toggle[sort],
                      icon:
                        direction === "desc" ? (
                          <ArrowDownWideNarrow aria-hidden="true" />
                        ) : (
                          <ArrowUpNarrowWide aria-hidden="true" />
                        ),
                      onClick: () =>
                        onSortChange(
                          withTagSortDirection(
                            sort,
                            direction === "asc" ? "desc" : "asc",
                          ),
                        ),
                    }
              }
              disabled={sortDisabled}
            />
          ),
        },
      ]}
      viewLabel={t.tags.sort.compact}
    >
      <FilterPopover
        count={filterCount}
        disabled={filterDisabled}
        canClear={filterCount > 0}
        onClear={onClearFilters}
        triggerRef={filterRef}
      >
        <div className="flex flex-col gap-3">
          <FilterCheckbox
            checked={tentativeOnly}
            onCheckedChange={onTentativeOnlyChange}
            label={t.tags.tentativeOnly}
            hint={t.tags.tentativeOnlyHint}
          />
          <FilterCheckbox
            checked={unusedOnly}
            onCheckedChange={onUnusedOnlyChange}
            label={t.tags.unusedOnly}
            hint={t.tags.unusedOnlyHint}
          />
        </div>
      </FilterPopover>
    </Toolbar>
  );
}

import { Heart, Plus, SearchX } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";

import {
  type Credit,
  type CreditKind,
  type CreditSort,
  createNamedCredit,
} from "../api/credits";
import { updateFavorites } from "../api/favorites";
import { t, type UiText } from "../i18n";
import { creditNameReason } from "../lib/creditName";
import { trimWhiteSpace } from "../lib/textRules";
import NameDialog, { type NameDialogWords } from "../library/NameDialog";
import { writeListSortPreferences } from "../preferences/tagListPreferences";
import TopBarPortal from "../shell/TopBarPortal";
import { TagLoadFailed, TagStaleList } from "../recordList/TagListNotices";
import type { TagListSort } from "../api/tags";
import { useUrlCriteria } from "../recordList/useUrlCriteria";
import { AdminListToolbar } from "../recordList/AdminListToolbar";
import {
  type FetchRowsPage,
  moreIdle,
  usePagedRows,
  useSearchQuery,
} from "../recordList/usePagedRows";
import { CardGrid } from "../ui/patterns/card-grid";
import { EmptyState } from "../ui/patterns/empty-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { FilterCheckbox, FilterPopover } from "../videoList/FilterMenu";
import type { HistoryMode } from "../videoList/listCriteria";
import { useLoadMoreSentinel } from "../videoList/pageHooks";
import type { SortOption } from "../videoList/SortControls";
import { CreditListTail } from "./CreditListStates";
import {
  creditGridBody,
  type CreditGridBody,
  type CreditGridCriteria,
  creditGridCountText,
} from "./creditGrid";
import { kindTexts, sortStorageKey } from "./creditRows";

function scrollToTop() {
  if (window.scrollY > 0) window.scrollTo({ top: 0, behavior: "auto" });
}

function validateName(text: string) {
  return trimWhiteSpace(text) === "" ? t.creditName.required : creditNameReason(text);
}

const noPreviewReset = () => undefined;

interface CreditGridEmptyProps {
  body: CreditGridBody;
  /** The kind's own state for an empty filtered page; null or undefined shows the empty grid's. */
  filtered: ReactNode;
  emptyIcon: ReactNode;
  emptyTitle: UiText;
  noMatchesTitle: UiText;
  noFavorites: UiText;
  newButton: ReactNode;
  onClearSearch: () => void;
}

/** The state shown in place of the cards when the read page has none. */
function CreditGridEmpty({
  body,
  filtered,
  emptyIcon,
  emptyTitle,
  noMatchesTitle,
  noFavorites,
  newButton,
  onClearSearch,
}: CreditGridEmptyProps) {
  const ownFiltered = body === "filtered" && filtered != null;
  if (ownFiltered) return filtered;
  if (body === "none" || body === "filtered") {
    return (
      <EmptyState
        icon={emptyIcon}
        title={emptyTitle}
        description={t.credits.empty.description}
        action={newButton}
      />
    );
  }
  if (body === "noMatches") {
    return (
      <EmptyState
        icon={<SearchX aria-hidden="true" />}
        title={noMatchesTitle}
        action={
          <Button variant="outline" size="sm" onClick={onClearSearch}>
            {t.credits.clearSearch}
          </Button>
        }
      />
    );
  }
  if (body === "noFavorites") {
    return <EmptyState icon={<Heart aria-hidden="true" />} title={noFavorites} />;
  }
  return null;
}

export interface CreditGridPageProps<
  R extends Credit,
  C extends CreditGridCriteria,
  E extends CreditSort,
> {
  kind: CreditKind;
  fetchPage: FetchRowsPage<R, C>;
  /** The conditions before the URL is read; kept for the life of the page. */
  initialQuery: C & { search: string };
  parse: (params: URLSearchParams) => { criteria: C; hasExplicitSort: boolean };
  serialize: (criteria: C) => URLSearchParams;
  /** Sorts after the shared ones, such as the people's ages. */
  extraSorts?: readonly SortOption<E>[];
  /** The applied toolbar filters, `favorite` included; the search is shown in its input. */
  filterCount: (criteria: C) => number;
  /** What `Clear filters` applies. */
  noFilters: Partial<C>;
  /** The kind's own filters, after `Favorites only` in the filter popover. */
  filters?: (criteria: C, change: (next: Partial<C>) => void) => ReactNode;
  /** The state of an empty page under the kind's own filters; null shows the empty grid's. */
  filteredEmpty?: (query: C) => ReactNode;
  cardSize: "face" | "logo";
  emptyIcon: ReactNode;
  noFavorites: UiText;
  newDialog: NameDialogWords;
  pagePath: (id: number) => string;
  renderCard: (row: R, onToggleFavorite: (row: R) => Promise<unknown>) => ReactNode;
}

/** A grid of credit cards paged 100 at a time from the server; each card opens the credit's page. */
export function CreditGridPage<
  R extends Credit,
  C extends CreditGridCriteria,
  E extends CreditSort = never,
>({
  kind,
  fetchPage,
  initialQuery,
  parse,
  serialize,
  extraSorts,
  filterCount,
  noFilters,
  filters,
  filteredEmpty,
  cardSize,
  emptyIcon,
  noFavorites,
  newDialog,
  pagePath,
  renderCard,
}: CreditGridPageProps<R, C, E>) {
  const texts = kindTexts(kind);
  const navigate = useNavigate();
  const {
    page,
    loadError,
    firstPending,
    more,
    appliedSearch,
    load,
    reload,
    loadMore,
    retryMore,
    applyLocal,
  } = usePagedRows<R, C>(fetchPage, initialQuery, scrollToTop);
  const { criteria, apply } = useUrlCriteria(parse, serialize);
  const [creating, setCreating] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const { normalizedQuery, searchRef } = useSearchQuery(criteria.query);
  // The conditions are flat values, so their JSON decides a reload as each one would.
  const rowsKey = JSON.stringify({ ...criteria, query: normalizedQuery });
  useEffect(() => {
    void load({ ...(JSON.parse(rowsKey) as C), search: searchRef.current });
  }, [rowsKey, load, searchRef]);

  // The heart changes only after the server answers; a card stays until the next fetch.
  const toggleFavorite = useCallback(
    async (row: R) => {
      const next = !row.favorite;
      const ids =
        kind === "person" ? { personIds: [row.id] } : { organizationIds: [row.id] };
      await updateFavorites([], [], next, ids);
      applyLocal(
        (current) =>
          current && {
            ...current,
            rows: current.rows.map((item) =>
              item.id === row.id ? { ...item, favorite: next } : item,
            ),
          },
      );
    },
    [kind, applyLocal],
  );

  const body = creditGridBody(page, loadError, firstPending);
  const stale = page !== undefined && loadError !== null;
  const sentinel = useLoadMoreSentinel(
    body === "cards" && !stale && page?.nextCursor !== undefined && more.kind === "idle",
    loadMore,
    noPreviewReset,
  );
  const noRows = page === undefined || page.totalAll === 0;
  const applied = filterCount(criteria);

  function change(next: Partial<C>, mode: HistoryMode = "push") {
    apply(next, mode);
  }

  function changeSort(next: CreditSort) {
    change({ sort: next } as Partial<C>);
    writeListSortPreferences(sortStorageKey(kind), { sort: next });
  }

  function clearSearch() {
    change({ query: "" } as Partial<C>);
    searchInputRef.current?.focus();
  }

  const newButton = (
    <Button size="sm" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" />
      {texts.newButton}
    </Button>
  );

  return (
    <ListPage
      toolbar={
        <TopBarPortal>
          <AdminListToolbar<E>
            query={criteria.query}
            onQueryCommit={(next, mode) => change({ query: next } as Partial<C>, mode)}
            searchRef={searchInputRef}
            searchDisabled={page !== undefined && page.totalAll === 0}
            searchLabel={texts.search}
            searchPlaceholder={texts.search}
            sort={criteria.sort as TagListSort | E}
            onSortChange={changeSort}
            extraSorts={extraSorts}
            sortDisabled={noRows}
            compactSortName={`${kind}-compact-sort`}
          >
            <FilterPopover
              count={applied}
              disabled={noRows && applied === 0}
              canClear={applied > 0}
              onClear={() => change({ favorite: false, ...noFilters })}
            >
              <FilterCheckbox
                checked={criteria.favorite}
                onCheckedChange={(next) => change({ favorite: next } as Partial<C>)}
                label={t.list.filter.favoritesOnly}
              />
              {filters?.(criteria, (next) => change(next))}
            </FilterPopover>
          </AdminListToolbar>
        </TopBarPortal>
      }
      header={
        <PageHeader
          title={texts.title}
          count={
            <span role="status" aria-live="polite" className="whitespace-nowrap">
              {creditGridCountText(kind, page, filterCount)}
            </span>
          }
          actions={newButton}
        />
      }
    >
      {body === "loading" && (
        <LoadingState
          label={t.credits.loading}
          layout="grid"
          size={cardSize}
          count={12}
        />
      )}
      {stale && loadError !== null && (
        <TagStaleList
          reason={loadError}
          pending={firstPending}
          onRetry={() => void reload()}
          message={texts.staleList}
        />
      )}
      {body === "failed" && loadError !== null && (
        <TagLoadFailed
          pending={firstPending}
          onRetry={() => void reload()}
          title={texts.loadFailed(loadError)}
        />
      )}
      <CreditGridEmpty
        body={body}
        filtered={body === "filtered" ? filteredEmpty?.(page!.query) : undefined}
        emptyIcon={emptyIcon}
        emptyTitle={texts.emptyTitle}
        noMatchesTitle={texts.noMatches(appliedSearch)}
        noFavorites={noFavorites}
        newButton={newButton}
        onClearSearch={clearSearch}
      />
      {body === "cards" && page !== undefined && (
        <>
          <CardGrid size={cardSize}>
            {page.rows.map((row) => renderCard(row, toggleFavorite))}
          </CardGrid>
          <CreditListTail
            kind={kind}
            tail={stale ? moreIdle : more}
            onRetryMore={retryMore}
            onReload={() => void reload()}
          />
          <div ref={sentinel} aria-hidden="true" className="h-px" />
        </>
      )}
      {creating && (
        <NameDialog
          words={newDialog}
          nameLabel={t.credits.create.label}
          nameRule={t.creditName.required}
          validate={validateName}
          send={(name) => createNamedCredit(kind, name)}
          onClose={() => setCreating(false)}
          onDone={(created) => void navigate(pagePath(created.id))}
        />
      )}
    </ListPage>
  );
}

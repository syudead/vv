import { Cake, CakeSlice, Heart, Plus, SearchX, Users } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { createCredit, type Person } from "../../api/credits";
import { updateFavorites } from "../../api/favorites";
import { t } from "../../i18n";
import { creditNameReason } from "../../lib/creditName";
import NameDialog from "../../library/NameDialog";
import { writeListSortPreferences } from "../../preferences/tagListPreferences";
import TopBarPortal from "../../shell/TopBarPortal";
import { TagLoadFailed, TagStaleList } from "../../tags/TagListNotices";
import { useUrlCriteria } from "../../tags/tagListUrl";
import { AdminListToolbar } from "../../tags/TagToolbar";
import { moreIdle, usePagedRows, useSearchQuery } from "../../tags/useTagPage";
import { CardGrid } from "../../ui/patterns/card-grid";
import { EmptyState } from "../../ui/patterns/empty-state";
import { ListPage } from "../../ui/patterns/list-page";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageHeader } from "../../ui/patterns/page-header";
import { Button } from "../../ui/shadcn/button";
import { FilterCheckbox, FilterPopover } from "../../videoList/FilterMenu";
import type { HistoryMode } from "../../videoList/listCriteria";
import { useLoadMoreSentinel } from "../../videoList/pageHooks";
import type { SortOption } from "../../videoList/SortControls";
import { CreditListTail } from "../CreditListStates";
import { sortStorageKey } from "../creditRows";
import { useLocalToday } from "../person/useLocalToday";
import {
  type AgeSort,
  fetchPeoplePage,
  isStalePeople,
  parsePeopleCriteria,
  peopleBody,
  peopleCountText,
  type PeopleCriteria,
  type PeopleRowsQuery,
  readPeopleSort,
  serializePeopleCriteria,
} from "./peopleCriteria";
import { PersonCard } from "./PersonCard";

const texts = t.credits.person;
const grid = t.credits.peopleGrid;

const ageSorts: readonly SortOption<AgeSort>[] = [
  { value: "ageAsc", label: grid.youngestFirst, icon: CakeSlice },
  { value: "ageDesc", label: grid.oldestFirst, icon: Cake },
];

const initialQuery: PeopleRowsQuery & { search: string } = {
  query: "",
  search: "",
  sort: "name",
  favorite: false,
};

function scrollToTop() {
  if (window.scrollY > 0) window.scrollTo({ top: 0, behavior: "auto" });
}

function validateName(text: string) {
  return text.trim() === "" ? t.creditName.required : creditNameReason(text);
}

const noPreviewReset = () => undefined;

/** `/people`: a grid of face cards paged 100 at a time; each card opens the person page. */
export default function PeoplePage() {
  const navigate = useNavigate();
  const today = useLocalToday();
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
  } = usePagedRows<Person, PeopleRowsQuery>(fetchPeoplePage, initialQuery, scrollToTop);
  const [preferredSort] = useState(readPeopleSort);
  const { criteria, apply } = useUrlCriteria(
    (params) => parsePeopleCriteria(params, preferredSort),
    serializePeopleCriteria,
  );
  const { query: search, sort, favorite } = criteria;
  const [creating, setCreating] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const { normalizedQuery, searchRef } = useSearchQuery(search);
  useEffect(() => {
    void load({ query: normalizedQuery, search: searchRef.current, sort, favorite });
  }, [normalizedQuery, sort, favorite, load, searchRef]);

  // The heart changes only after the server answers; a card stays until the next fetch.
  const toggleFavorite = useCallback(
    async (person: Person) => {
      const next = !person.favorite;
      await updateFavorites([], [], next, { personIds: [person.id] });
      applyLocal(
        (current) =>
          current && {
            ...current,
            rows: current.rows.map((row) =>
              row.id === person.id ? { ...row, favorite: next } : row,
            ),
          },
      );
    },
    [applyLocal],
  );

  const body = peopleBody(page, loadError, firstPending);
  const stale = isStalePeople(page, loadError);
  const sentinel = useLoadMoreSentinel(
    body === "cards" && !stale && page?.nextCursor !== undefined && more.kind === "idle",
    loadMore,
    noPreviewReset,
  );
  const noPeople = page === undefined || page.totalAll === 0;

  function change(next: Partial<PeopleCriteria>, mode: HistoryMode = "push") {
    apply(next, mode);
  }

  function changeSort(next: PeopleCriteria["sort"]) {
    change({ sort: next });
    writeListSortPreferences(sortStorageKey("person"), { sort: next });
  }

  function clearSearch() {
    change({ query: "" });
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
          <AdminListToolbar<AgeSort>
            query={search}
            onQueryCommit={(next, mode) => change({ query: next }, mode)}
            searchRef={searchInputRef}
            searchDisabled={page !== undefined && page.totalAll === 0}
            searchLabel={texts.search}
            searchPlaceholder={texts.search}
            sort={sort}
            onSortChange={changeSort}
            extraSorts={ageSorts}
            sortDisabled={noPeople}
            compactSortName="person-compact-sort"
          >
            <FilterPopover
              count={favorite ? 1 : 0}
              disabled={noPeople && !favorite}
              canClear={favorite}
              onClear={() => change({ favorite: false })}
            >
              <FilterCheckbox
                checked={favorite}
                onCheckedChange={(next) => change({ favorite: next })}
                label={t.list.filter.favoritesOnly}
              />
            </FilterPopover>
          </AdminListToolbar>
        </TopBarPortal>
      }
      header={
        <PageHeader
          title={texts.title}
          count={
            <span role="status" aria-live="polite" className="whitespace-nowrap">
              {peopleCountText(page)}
            </span>
          }
          actions={newButton}
        />
      }
    >
      {body === "loading" && (
        <LoadingState label={t.credits.loading} layout="grid" size="face" count={12} />
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
      {body === "noPeople" && (
        <EmptyState
          icon={<Users aria-hidden="true" />}
          title={texts.emptyTitle}
          description={t.credits.empty.description}
          action={newButton}
        />
      )}
      {body === "noMatches" && (
        <EmptyState
          icon={<SearchX aria-hidden="true" />}
          title={texts.noMatches(appliedSearch)}
          action={
            <Button variant="outline" size="sm" onClick={clearSearch}>
              {t.credits.clearSearch}
            </Button>
          }
        />
      )}
      {body === "noFavorites" && (
        <EmptyState icon={<Heart aria-hidden="true" />} title={grid.noFavorites} />
      )}
      {body === "cards" && page !== undefined && (
        <>
          <CardGrid size="face">
            {page.rows.map((person) => (
              <PersonCard
                key={person.id}
                person={person}
                today={today}
                onToggleFavorite={toggleFavorite}
              />
            ))}
          </CardGrid>
          <CreditListTail
            kind="person"
            tail={stale ? moreIdle : more}
            onRetryMore={retryMore}
            onReload={() => void reload()}
          />
          <div ref={sentinel} aria-hidden="true" className="h-px" />
        </>
      )}
      {creating && (
        <NameDialog
          words={grid.newDialog}
          nameLabel={t.credits.create.label}
          nameRule={t.creditName.required}
          validate={validateName}
          send={(name) => createCredit("person", { name })}
          onClose={() => setCreating(false)}
          onDone={(created) => void navigate(`/people/${String(created.id)}`)}
        />
      )}
    </ListPage>
  );
}

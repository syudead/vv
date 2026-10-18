import { Search, X } from "lucide-react";
import { type RefObject, useEffect, useRef, useState } from "react";

import type { ActivityFeature, ActivityStatus } from "../api/activity";
import { t, type UiText } from "../i18n";
import { FilterDropdownButton } from "../ui/FilterDropdownButton";
import { Toolbar } from "../ui/patterns/toolbar";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenuCheckboxItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
} from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import {
  ACTIVITY_QUERY_LIMIT,
  type ActivityCriteria,
  type ActivityPeriod,
  activityDeciders,
  activityPeriods,
  activityStatuses,
  hasActivityConditions,
  normalizeActivityQuery,
  ownerFeatures,
  runFeatures,
} from "./activityCriteria";
import type { ActivityLogState } from "./useActivityLog";

const allValue = "all";

function MenuCount({ count, alert = false }: { count: number; alert?: boolean }) {
  return (
    <span
      className={
        alert && count > 0
          ? "ml-auto pl-4 text-xs text-destructive tabular-nums"
          : "ml-auto pl-4 text-xs text-muted-foreground tabular-nums"
      }
    >
      {t.activity.filter.count(count)}
    </span>
  );
}

export function featureValue(features: readonly ActivityFeature[]): UiText {
  const [first] = features;
  if (first === undefined) return t.activity.filter.all;
  const name = t.activity.featureValue[first];
  return features.length === 1
    ? name
    : t.activity.filter.several(name, features.length - 1);
}

function statusValue(status: ActivityStatus | undefined): UiText {
  return status === undefined ? t.activity.filter.all : t.activity.statusValue[status];
}

function SearchField({
  query,
  onQuery,
  inputRef,
}: {
  query: string;
  onQuery: (value: string, mode: HistoryMode) => void;
  inputRef?: RefObject<HTMLInputElement | null>;
}) {
  return (
    <SearchBox
      query={query}
      onCommit={(next, mode) => onQuery(normalizeActivityQuery(next), mode)}
      inputRef={inputRef}
      label={t.activity.search.label}
      placeholder={t.activity.search.label}
      syntaxHelp={false}
      maxLength={ACTIVITY_QUERY_LIMIT}
    />
  );
}

export interface ActivityToolbarProps {
  criteria: ActivityCriteria;
  state: ActivityLogState;
  /** History mode defaults to push. */
  onChange: (next: ActivityCriteria, mode?: HistoryMode) => void;
}

export function ActivityToolbar({ criteria, state, onChange }: ActivityToolbarProps) {
  const counts = state.kind === "ready" ? state.counts : undefined;
  const features = state.kind === "ready" ? state.features : undefined;
  const [searchOpen, setSearchOpen] = useState(false);
  const overlayInput = useRef<HTMLInputElement>(null);
  const onQuery = (q: string, mode: HistoryMode) => onChange({ ...criteria, q }, mode);
  const clear = () => onChange({ features: [], period: "any", q: "" });

  useEffect(() => {
    if (searchOpen) overlayInput.current?.focus();
  }, [searchOpen]);

  const toggleFeature = (feature: ActivityFeature, checked: boolean) => {
    const rest = criteria.features.filter((item) => item !== feature);
    onChange({ ...criteria, features: checked ? [...rest, feature] : rest });
  };

  const featureItem = (feature: ActivityFeature) => (
    <DropdownMenuCheckboxItem
      key={feature}
      checked={criteria.features.includes(feature)}
      onSelect={(event) => event.preventDefault()}
      onCheckedChange={(checked) => toggleFeature(feature, checked)}
    >
      {t.activity.featureValue[feature]}
      {features !== undefined && <MenuCount count={features[feature]} />}
    </DropdownMenuCheckboxItem>
  );

  return (
    <Toolbar nowrap search={<SearchField query={criteria.q} onQuery={onQuery} />}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="icon-sm"
            className="md:hidden"
            aria-label={t.activity.search.label}
            data-active={criteria.q === "" ? undefined : "true"}
            onClick={() => setSearchOpen(true)}
          >
            <Search aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t.activity.search.label}</TooltipContent>
      </Tooltip>
      {searchOpen && (
        <div
          className="absolute inset-0 z-10 flex items-center gap-2 bg-background md:hidden"
          onBlur={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget)) setSearchOpen(false);
          }}
        >
          <div className="min-w-0 flex-1">
            <SearchField query={criteria.q} onQuery={onQuery} inputRef={overlayInput} />
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t.common.close}
            onClick={() => setSearchOpen(false)}
          >
            <X aria-hidden="true" />
          </Button>
        </div>
      )}
      <FilterDropdownButton
        name={t.activity.filter.status}
        value={statusValue(criteria.status)}
        active={criteria.status !== undefined}
      >
        <DropdownMenuRadioGroup
          value={criteria.status ?? allValue}
          onValueChange={(value) => {
            const { status: _status, ...rest } = criteria;
            onChange(
              value === allValue ? rest : { ...rest, status: value as ActivityStatus },
            );
          }}
        >
          <DropdownMenuRadioItem value={allValue}>
            {t.activity.filter.all}
          </DropdownMenuRadioItem>
          {activityStatuses.map((status) => (
            <DropdownMenuRadioItem key={status} value={status}>
              {t.activity.statusValue[status]}
              {counts !== undefined && (
                <MenuCount count={counts[status]} alert={status === "attention"} />
              )}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </FilterDropdownButton>
      <FilterDropdownButton
        name={t.activity.filter.feature}
        value={featureValue(criteria.features)}
        active={criteria.features.length > 0}
      >
        {runFeatures.map(featureItem)}
        <DropdownMenuSeparator />
        {ownerFeatures.map(featureItem)}
      </FilterDropdownButton>
      <FilterDropdownButton
        name={t.activity.filter.decider}
        value={
          criteria.decider === undefined
            ? t.activity.filter.all
            : t.activity.deciderValue[criteria.decider]
        }
        active={criteria.decider !== undefined}
      >
        <DropdownMenuRadioGroup
          value={criteria.decider ?? allValue}
          onValueChange={(value) => {
            const { decider: _decider, ...rest } = criteria;
            onChange(
              value === allValue
                ? rest
                : { ...rest, decider: value as NonNullable<ActivityCriteria["decider"]> },
            );
          }}
        >
          <DropdownMenuRadioItem value={allValue}>
            {t.activity.filter.all}
          </DropdownMenuRadioItem>
          {activityDeciders.map((decider) => (
            <DropdownMenuRadioItem key={decider} value={decider}>
              {t.activity.deciderValue[decider]}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </FilterDropdownButton>
      <FilterDropdownButton
        name={t.activity.filter.period}
        value={t.activity.periodValue[criteria.period]}
        active={criteria.period !== "any"}
        align="end"
      >
        <DropdownMenuRadioGroup
          value={criteria.period}
          onValueChange={(value) =>
            onChange({ ...criteria, period: value as ActivityPeriod })
          }
        >
          {activityPeriods.map((period) => (
            <DropdownMenuRadioItem key={period} value={period}>
              {t.activity.periodValue[period]}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </FilterDropdownButton>
      {hasActivityConditions(criteria) && (
        <Button variant="ghost" size="sm" className="shrink-0" onClick={clear}>
          {t.activity.filter.clear}
        </Button>
      )}
    </Toolbar>
  );
}

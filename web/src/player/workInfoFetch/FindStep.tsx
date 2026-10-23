import { CircleAlert, ExternalLink } from "lucide-react";
import { type RefObject, useId, useRef } from "react";
import { Link } from "react-router";

import { RequestFailed } from "../../api/client";
import type { WorkInfoCandidate, WorkInfoPlugin } from "../../api/workInfoPlugins";
import { errorText, t } from "../../i18n";
import { Alert, AlertDescription, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Field, FieldDescription, FieldError, FieldLabel } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/shadcn/select";
import { Spinner } from "../../ui/shadcn/spinner";
import { ToggleGroup, ToggleGroupItem } from "../../ui/shadcn/toggle-group";
import ActionButton from "../ActionButton";
import {
  canChooseFindBy,
  canFetch,
  type FindBy,
  hostOf,
  missingSettingLabels,
  pluginLabel,
  queryLabel,
  type RunOperation,
} from "./fetchRules";

export const WORK_INFO_PLUGIN_SETTINGS = "/settings/plugins#work-info-plugins";

const MAX_CANDIDATES = 50;

export interface RunFailure {
  operation: RunOperation;
  error: unknown;
  plugin: WorkInfoPlugin | undefined;
}

export interface Running {
  operation: RunOperation;
  id?: string;
}

export function PluginField({
  plugins,
  plugin,
  disabled,
  onPlugin,
}: {
  plugins: readonly WorkInfoPlugin[];
  plugin: WorkInfoPlugin | undefined;
  disabled: boolean;
  onPlugin: (id: number) => void;
}) {
  const text = t.player.workInfoFetch;
  const id = useId();
  if (plugins.length <= 1) {
    return (
      <p className="flex flex-wrap gap-x-2 text-sm">
        <span className="font-medium">{text.plugin}</span>
        <span className="break-words">
          {plugin === undefined ? "" : pluginLabel(plugin)}
        </span>
      </p>
    );
  }
  return (
    <Field>
      <FieldLabel htmlFor={id}>{text.plugin}</FieldLabel>
      <Select
        value={plugin === undefined ? "" : String(plugin.id)}
        onValueChange={(value) => onPlugin(Number(value))}
        disabled={disabled}
      >
        <SelectTrigger id={id} size="sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {plugins.map((item) => (
            <SelectItem key={item.id} value={String(item.id)}>
              {pluginLabel(item)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}

const FIND_BY: readonly FindBy[] = ["words", "id"];

function FindByField({
  by,
  disabled,
  onBy,
}: {
  by: FindBy;
  disabled: boolean;
  onBy: (by: FindBy) => void;
}) {
  const text = t.player.workInfoFetch;
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      aria-label={text.findBy}
      className="self-start"
      value={by}
      disabled={disabled}
      onValueChange={(value) => {
        if (value === "" || value === by) return;
        onBy(value as FindBy);
      }}
    >
      {FIND_BY.map((option) => (
        <ToggleGroupItem key={option} value={option} className="flex-none px-3">
          {text.findByOptions[option]}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

export function FailureAlert({
  failure,
  settingsPath = WORK_INFO_PLUGIN_SETTINGS,
}: {
  failure: RunFailure;
  settingsPath?: string;
}) {
  const text = t.player.workInfoFetch;
  const { error, plugin } = failure;
  if (error instanceof RequestFailed && error.code === "plugin_settings_missing") {
    const labels = missingSettingLabels(plugin, error.settingKeys ?? []);
    return (
      <Alert variant="warning">
        <CircleAlert aria-hidden="true" />
        <AlertTitle>
          {text.settingsMissing(plugin === undefined ? "" : pluginLabel(plugin), labels)}
        </AlertTitle>
        <AlertDescription>
          <Button variant="link" size="sm" className="h-auto px-0 py-0" asChild>
            <Link to={settingsPath}>{text.openSettings}</Link>
          </Button>
        </AlertDescription>
      </Alert>
    );
  }
  const reason = errorText(error);
  const message = error instanceof RequestFailed ? error.pluginMessage : undefined;
  return (
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>
        {failure.operation === "search"
          ? text.searchFailed(reason)
          : text.fetchFailed(reason)}
      </AlertTitle>
      {message !== undefined && message !== "" && (
        <AlertDescription>
          <p className="break-words text-muted-foreground">{text.pluginSaid(message)}</p>
        </AlertDescription>
      )}
    </Alert>
  );
}

/** A candidate as the list shows it; the key is what choosing it passes back. */
export interface CandidateView {
  key: string;
  title: string;
  detail?: string;
  url: string;
}

function CandidateText({ candidate }: { candidate: CandidateView }) {
  return (
    <>
      <span
        title={candidate.title}
        className="line-clamp-2 text-sm wrap-anywhere text-foreground"
      >
        {candidate.title}
      </span>
      {candidate.detail !== undefined && candidate.detail !== "" && (
        <span className="w-full truncate text-xs text-muted-foreground">
          {candidate.detail}
        </span>
      )}
      <span className="w-full truncate text-xs text-muted-foreground">
        {hostOf(candidate.url)}
      </span>
    </>
  );
}

/** Unchoosable candidates expose links without selectable rows. Candidate URLs are navigation only. */
function CandidateRow({
  candidate,
  choosable,
  chosen,
  busy,
  onChoose,
}: {
  candidate: CandidateView;
  choosable: boolean;
  chosen: boolean;
  busy: boolean;
  onChoose: (key: string) => void;
}) {
  const text = t.player.workInfoFetch;
  const layout =
    "h-auto min-w-0 flex-1 flex-col items-start gap-0.5 px-2 py-1.5 text-left";
  return (
    <li className="flex items-center gap-1 py-1">
      {choosable ? (
        <Button
          variant="ghost"
          className={`${layout} font-normal whitespace-normal`}
          aria-disabled={busy || undefined}
          onClick={() => {
            if (!busy) onChoose(candidate.key);
          }}
        >
          <CandidateText candidate={candidate} />
        </Button>
      ) : (
        <div className={`flex ${layout}`}>
          <CandidateText candidate={candidate} />
        </div>
      )}
      {chosen && <Spinner aria-hidden="true" role={undefined} />}
      <ActionButton label={text.openPage} asChild>
        <a href={candidate.url} target="_blank" rel="noopener noreferrer">
          <ExternalLink aria-hidden="true" />
        </a>
      </ActionButton>
    </li>
  );
}

/** The first 50 candidates under their count, or under heading; the caller shows the empty state. */
export function CandidateList({
  candidates,
  heading,
  choosable,
  chosenKey,
  busy,
  onChoose,
}: {
  candidates: readonly CandidateView[];
  heading?: string;
  choosable: boolean;
  chosenKey: string | undefined;
  busy: boolean;
  onChoose: (key: string) => void;
}) {
  const text = t.player.workInfoFetch;
  const headingId = useId();
  const shown = candidates.slice(0, MAX_CANDIDATES);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-1">
      <h3 id={headingId} className="text-sm text-muted-foreground">
        {heading ?? text.matches(shown.length)}
      </h3>
      <ul className="flex flex-col divide-y divide-border border-y border-border">
        {shown.map((candidate) => (
          <CandidateRow
            key={candidate.key}
            candidate={candidate}
            choosable={choosable}
            chosen={chosenKey === candidate.key}
            busy={busy}
            onChoose={onChoose}
          />
        ))}
      </ul>
    </section>
  );
}

function Candidates({
  plugin,
  candidates,
  running,
  onChoose,
}: {
  plugin: WorkInfoPlugin | undefined;
  candidates: readonly WorkInfoCandidate[];
  running: Running | null;
  onChoose: (id: string) => void;
}) {
  const text = t.player.workInfoFetch;
  const choosable = plugin !== undefined && canFetch(plugin);
  if (candidates.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {choosable ? text.noMatchesFetch : text.noMatches}
      </p>
    );
  }
  return (
    <CandidateList
      candidates={candidates.map((candidate) => ({ ...candidate, key: candidate.id }))}
      choosable={choosable}
      chosenKey={running?.id}
      busy={running !== null}
      onChoose={onChoose}
    />
  );
}

/** A prefilled query, selected on its first focus so typing replaces it without reselecting later. An error replaces the description. */
export function QueryField({
  label,
  value,
  busy,
  inputRef,
  autoFocus,
  description,
  error,
  onChange,
}: {
  label: string;
  value: string;
  busy: boolean;
  inputRef?: RefObject<HTMLInputElement | null>;
  autoFocus?: boolean;
  description?: string;
  error?: string;
  onChange: (value: string) => void;
}) {
  const id = useId();
  const noteId = useId();
  const selectedOnce = useRef(false);
  const invalid = error !== undefined;
  const hasNote = invalid || description !== undefined;
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        ref={inputRef}
        id={id}
        value={value}
        autoFocus={autoFocus}
        readOnly={busy}
        aria-disabled={busy || undefined}
        aria-invalid={invalid || undefined}
        aria-describedby={hasNote ? noteId : undefined}
        autoComplete="off"
        onFocus={(event) => {
          if (selectedOnce.current) return;
          selectedOnce.current = true;
          event.currentTarget.select();
        }}
        onChange={(event) => onChange(event.target.value)}
      />
      {invalid ? (
        <FieldError id={noteId}>{error}</FieldError>
      ) : (
        description !== undefined && (
          <FieldDescription id={noteId}>{description}</FieldDescription>
        )
      )}
    </Field>
  );
}

export default function FindStep({
  plugins,
  plugin,
  by,
  query,
  running,
  failure,
  candidates,
  queryRef,
  onPlugin,
  onBy,
  onQuery,
  onChoose,
}: {
  plugins: readonly WorkInfoPlugin[];
  plugin: WorkInfoPlugin | undefined;
  by: FindBy;
  query: string;
  running: Running | null;
  failure: RunFailure | null;
  candidates: readonly WorkInfoCandidate[] | null;
  queryRef: RefObject<HTMLInputElement | null>;
  onPlugin: (id: number) => void;
  onBy: (by: FindBy) => void;
  onQuery: (query: string) => void;
  onChoose: (id: string) => void;
}) {
  const text = t.player.workInfoFetch;
  const busy = running !== null;
  return (
    <>
      <PluginField
        plugins={plugins}
        plugin={plugin}
        disabled={busy}
        onPlugin={onPlugin}
      />
      {plugin !== undefined && canChooseFindBy(plugin) && (
        <FindByField by={by} disabled={busy} onBy={onBy} />
      )}
      <QueryField
        label={plugin === undefined ? text.querySearch : queryLabel(plugin, by)}
        value={query}
        busy={busy}
        inputRef={queryRef}
        onChange={onQuery}
      />
      {failure !== null && <FailureAlert failure={failure} />}
      {candidates !== null && (
        <Candidates
          plugin={plugin}
          candidates={candidates}
          running={running}
          onChoose={onChoose}
        />
      )}
    </>
  );
}

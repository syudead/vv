import { CircleAlert, ExternalLink } from "lucide-react";
import { type RefObject, useId, useRef } from "react";
import { Link } from "react-router";

import { RequestFailed } from "../../api/client";
import type { WorkInfoCandidate, WorkInfoPlugin } from "../../api/workInfoPlugins";
import { errorText, t } from "../../i18n";
import { Alert, AlertDescription, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Field, FieldLabel } from "../../ui/shadcn/field";
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

/** WORK_INFO_PLUGIN_SETTINGS は Settings の Plugins のページの「Work-info plugins」の節である。 */
export const WORK_INFO_PLUGIN_SETTINGS = "/settings/plugins#work-info-plugins";

/** MAX_CANDIDATES は出す候補の数の上限である（ui-design「Candidates」）。 */
const MAX_CANDIDATES = 50;

/** RunFailure は検索か取得の失敗と、そのときのプラグインである。 */
export interface RunFailure {
  operation: RunOperation;
  error: unknown;
  plugin: WorkInfoPlugin | undefined;
}

/** Running は走っている検索か取得で、`url` は候補の行から取っているときのその URL である。 */
export interface Running {
  operation: RunOperation;
  url?: string;
}

function PluginField({
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

/**
 * FindByField は語を検索の語にするか、取得に渡す作品の ID にするかの選択で、search と fetch の両方を
 * 宣言するプラグインにだけ出す。押している選択肢は外せない。
 */
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

/** FailureAlert は検索か取得の失敗である（ui-design「Failures」）。 */
function FailureAlert({ failure }: { failure: RunFailure }) {
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
            <Link to={WORK_INFO_PLUGIN_SETTINGS}>{text.openSettings}</Link>
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

function CandidateText({ candidate }: { candidate: WorkInfoCandidate }) {
  return (
    <>
      <span title={candidate.title} className="line-clamp-2 text-sm text-foreground">
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

/**
 * CandidateRow は候補の 1 行である。行を押すとその URL を取得するので、取得できない
 * （検索だけの）プラグインでは行を押せる形にせず、文と `Open page` だけを出す。
 */
function CandidateRow({
  candidate,
  choosable,
  running,
  onChoose,
}: {
  candidate: WorkInfoCandidate;
  choosable: boolean;
  running: Running | null;
  onChoose: (url: string) => void;
}) {
  const text = t.player.workInfoFetch;
  const chosen = running?.url === candidate.url;
  const busy = running !== null;
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
            if (!busy) onChoose(candidate.url);
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

function Candidates({
  plugin,
  candidates,
  running,
  onChoose,
}: {
  plugin: WorkInfoPlugin | undefined;
  candidates: readonly WorkInfoCandidate[];
  running: Running | null;
  onChoose: (url: string) => void;
}) {
  const text = t.player.workInfoFetch;
  const headingId = useId();
  if (candidates.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {plugin !== undefined && canFetch(plugin) ? text.noMatchesFetch : text.noMatches}
      </p>
    );
  }
  const shown = candidates.slice(0, MAX_CANDIDATES);
  const choosable = plugin !== undefined && canFetch(plugin);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-1">
      <h3 id={headingId} className="text-sm text-muted-foreground">
        {text.matches(shown.length)}
      </h3>
      <ul className="flex flex-col divide-y divide-border border-y border-border">
        {shown.map((candidate) => (
          <CandidateRow
            key={candidate.url}
            candidate={candidate}
            choosable={choosable}
            running={running}
            onChoose={onChoose}
          />
        ))}
      </ul>
    </section>
  );
}

/**
 * FindStep は取得のダイアログの最初の段で、プラグインの選択、語と ID の選択（両方できるプラグインだけ）、
 * 語か ID か URL の 1 つの欄、失敗、候補の一覧を縦に並べる（ui-design「Find」「Candidates」「Failures」）。
 */
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
  onChoose: (url: string) => void;
}) {
  const text = t.player.workInfoFetch;
  const queryId = useId();
  // 開いて最初にフォーカスを受けたときだけ、題名を選んだ状態にして打てば置き換わるようにする。
  const selectedOnce = useRef(false);
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
      <Field>
        <FieldLabel htmlFor={queryId}>
          {plugin === undefined ? text.queryBoth : queryLabel(plugin, by)}
        </FieldLabel>
        <Input
          ref={queryRef}
          id={queryId}
          value={query}
          readOnly={busy}
          aria-disabled={busy || undefined}
          autoComplete="off"
          onFocus={(event) => {
            if (selectedOnce.current) return;
            selectedOnce.current = true;
            event.currentTarget.select();
          }}
          onChange={(event) => onQuery(event.target.value)}
        />
      </Field>
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

import { personFaceSrc } from "../../api/credits";
import type { PersonPluginItem } from "../../api/personPluginFetches";
import { personLookupImageUrl, type PersonLookup } from "../../api/workInfoPlugins";
import PersonCompare from "../../credits/person/pluginFetch/PersonCompare";
import { errorText, t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { PersonFace } from "../../ui/PersonFace";
import { type LookupState, lookupCurrent } from "./lookupRules";

export interface LookupBlockProps {
  videoId: number;
  fetchId: string;
  index: number;
  lookup: PersonLookup;
  names: readonly string[];
  state: LookupState;
  /** False while none of the lookup's role rows is checked. */
  enabled: boolean;
  /** While the apply runs or the result is stale. */
  locked: boolean;
  onToggleItem: (index: number, item: PersonPluginItem, on: boolean) => void;
  onRetry: (index: number) => void;
}

function Body({ props, name }: { props: LookupBlockProps; name: string }) {
  const text = t.player.workInfoFetch;
  const { state, index } = props;
  if (state.status === "looking")
    return (
      <p role="status" className="flex items-center gap-2 text-muted-foreground">
        <Spinner aria-hidden="true" role={undefined} />
        {text.lookingUp}
      </p>
    );
  if (state.status === "changed")
    return <p className="text-muted-foreground">{text.lookupChanged}</p>;
  if (state.status === "failed")
    return (
      <div className="flex flex-wrap items-baseline gap-x-2">
        <p role="alert" className="min-w-0 break-words text-destructive">
          {text.lookupFailed(name, errorText(state.error))}
        </p>
        <Button
          variant="link"
          size="sm"
          className="h-auto px-0 py-0"
          disabled={props.locked}
          onClick={() => props.onRetry(index)}
        >
          {text.tryAgain}
        </Button>
      </div>
    );
  const { result } = state;
  return (
    <PersonCompare
      current={lookupCurrent(result)}
      fetched={result.person}
      nowFaceSrc={result.current === null ? undefined : personFaceSrc(result.current)}
      fetchedFaceSrc={
        result.person.hasImage === true
          ? personLookupImageUrl(props.videoId, props.fetchId, index)
          : undefined
      }
      selection={state.items}
      locked={props.locked || !props.enabled}
      inBlock
      onToggle={(item, on) => props.onToggleItem(index, item, on)}
    />
  );
}

/** What a person plugin found for a fetched person, under the person's chip and quieter than it. */
export default function LookupBlock(props: LookupBlockProps) {
  const name = props.names.join(" · ");
  const { state } = props;
  const face =
    state.status === "shown" && state.result.person.hasImage === true
      ? personLookupImageUrl(props.videoId, props.fetchId, props.index)
      : undefined;
  return (
    <section
      aria-label={name}
      className="mt-1 flex min-w-0 basis-full flex-col gap-1 border-l-2 border-border pl-3"
    >
      <div className="flex min-w-0 items-center gap-2">
        <PersonFace shape="compare" name="" src={face} />
        <span className="min-w-0 truncate font-medium" title={name}>
          {name}
        </span>
        <span className="shrink-0 text-xs text-muted-foreground">
          {t.player.workInfoFetch.lookupSource(props.lookup.pluginName)}
        </span>
      </div>
      <Body props={props} name={name} />
    </section>
  );
}

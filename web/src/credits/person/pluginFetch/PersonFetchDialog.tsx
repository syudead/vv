import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";

import { type Person, personFaceSrc } from "../../../api/credits";
import {
  type PersonPluginCandidate,
  personPluginFetchImageUrl,
} from "../../../api/personPluginFetches";
import type { WorkInfoPlugin } from "../../../api/workInfoPlugins";
import { t } from "../../../i18n";
import { CompareNotices } from "../../../player/workInfoFetch/CompareStep";
import { busyLabel, ResultStep } from "../../../player/workInfoFetch/FetchDialog";
import { pluginLabel } from "../../../player/workInfoFetch/fetchRules";
import {
  CandidateList,
  type CandidateView,
  FailureAlert,
  PluginField,
  QueryField,
} from "../../../player/workInfoFetch/FindStep";
import { FormDialog } from "../../../ui/patterns/form-dialog";
import { Button } from "../../../ui/shadcn/button";
import PersonCompare from "./PersonCompare";
import {
  candidateKey,
  type PersonFetchFlow,
  usePersonFetchFlow,
} from "./usePersonFetchFlow";

export const PERSON_PLUGIN_SETTINGS = "/settings/plugins#person-plugins";

const text = t.credits.personPage.pluginFetch;

function candidateView(candidate: PersonPluginCandidate): CandidateView {
  const sourceId = text.candidateId(candidate.source, candidate.id);
  return {
    key: candidateKey(candidate),
    title: candidate.name,
    detail:
      candidate.detail === undefined || candidate.detail === ""
        ? sourceId
        : text.candidateDetail(candidate.detail, sourceId),
    url: candidate.url,
  };
}

function NameSearch({ flow }: { flow: PersonFetchFlow }) {
  const busy = flow.running !== null;
  const candidates = flow.candidates;
  return (
    <>
      <p className="text-sm text-muted-foreground">
        {text.noAcceptedId(flow.plugin === undefined ? "" : pluginLabel(flow.plugin))}
      </p>
      <QueryField
        label={text.name}
        value={flow.query}
        busy={busy}
        autoFocus
        onChange={flow.changeQuery}
      />
      {candidates !== null &&
        (candidates.length === 0 ? (
          <p className="text-sm text-muted-foreground">{text.noMatches}</p>
        ) : (
          <CandidateList
            candidates={candidates.map(candidateView)}
            choosable
            chosenKey={flow.running?.key}
            busy={busy}
            onChoose={(key) => {
              const chosen = candidates.find((item) => candidateKey(item) === key);
              if (chosen !== undefined)
                flow.choose({ source: chosen.source, id: chosen.id });
            }}
          />
        ))}
    </>
  );
}

function FindStep({
  flow,
  plugins,
}: {
  flow: PersonFetchFlow;
  plugins: readonly WorkInfoPlugin[];
}) {
  return (
    <>
      <PluginField
        plugins={plugins}
        plugin={flow.plugin}
        disabled={flow.running !== null}
        onPlugin={flow.choosePlugin}
      />
      {flow.byName && <NameSearch flow={flow} />}
      {flow.failure !== null && (
        <FailureAlert failure={flow.failure} settingsPath={PERSON_PLUGIN_SETTINGS} />
      )}
    </>
  );
}

function CompareStep({ flow, person }: { flow: PersonFetchFlow; person: Person }) {
  const fetched = flow.fetched;
  if (fetched === null) return null;
  const first = fetched.result.person.externalIds[0];
  return (
    <div className="flex flex-col gap-3">
      <p className="text-xs break-words text-muted-foreground">
        {text.source(
          pluginLabel(fetched.plugin),
          first?.source ?? "",
          first?.value ?? "",
        )}
      </p>
      {fetched.candidate !== undefined && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start"
          disabled={flow.applying || flow.running !== null}
          onClick={flow.back}
        >
          <ArrowLeft aria-hidden="true" />
          {t.player.workInfoFetch.back}
        </Button>
      )}
      <CompareNotices
        stale={flow.stale}
        refetching={flow.running !== null}
        failure={flow.applyFailure}
        onFetchAgain={flow.fetchAgain}
      />
      <PersonCompare
        current={person}
        fetched={fetched.result.person}
        nowFaceSrc={personFaceSrc(person)}
        fetchedFaceSrc={personPluginFetchImageUrl(person.id, fetched.result.fetchId)}
        selection={flow.selection}
        locked={flow.applying || flow.stale}
        onToggle={flow.toggle}
      />
    </div>
  );
}

function submitLabel(flow: PersonFetchFlow): ReactNode {
  const words = t.player.workInfoFetch;
  if (flow.step === "result") return words.done;
  if (flow.step === "compare")
    return flow.applying ? busyLabel(words.applying) : words.apply;
  if (flow.running !== null && flow.running.key === undefined)
    return busyLabel(
      flow.running.operation === "search" ? words.searching : words.fetching,
    );
  return flow.byName ? words.search : words.fetch;
}

function submitDisabled(flow: PersonFetchFlow): boolean {
  if (flow.step === "result") return false;
  if (flow.step === "compare")
    return flow.selection.length === 0 || flow.stale || flow.running !== null;
  if (flow.plugin === undefined || flow.running !== null) return true;
  return flow.byName && flow.query.trim() === "";
}

/** Fetch info…: a person plugin's result compared with the person, applied item by item. */
export default function PersonFetchDialog(props: {
  person: Person;
  plugins: readonly WorkInfoPlugin[];
  onReload: () => Promise<void>;
  onApplied: (person: Person) => void;
  onGone: () => void;
  onClose: () => void;
}) {
  const { person, plugins, onClose } = props;
  const flow = usePersonFetchFlow(props);
  return (
    <FormDialog
      open
      size="wide"
      onOpenChange={(open) => {
        if (open || flow.applying) return;
        flow.cancel();
        onClose();
      }}
      title={text.title(person.name)}
      description={t.player.workInfoFetch.description}
      submitLabel={submitLabel(flow)}
      cancelLabel={flow.step === "result" ? undefined : t.common.cancel}
      pending={flow.applying}
      submitDisabled={submitDisabled(flow)}
      onSubmit={(event) => {
        event.preventDefault();
        flow.submit();
      }}
    >
      {flow.step === "find" && <FindStep flow={flow} plugins={plugins} />}
      {flow.step === "compare" && <CompareStep flow={flow} person={person} />}
      {flow.step === "result" && <ResultStep lines={flow.skipped} />}
    </FormDialog>
  );
}

import { ArrowLeft, Search } from "lucide-react";
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
import { selectionEmpty } from "./personCompareRows";
import { isLink } from "../../../lib/typedLink";
import {
  candidateKey,
  type LinkRefusal,
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

/** A link row reads as the path, query and fragment as stored, over the host the list draws. */
function linkView(link: string): CandidateView {
  const rest = link.replace(/^[a-z][a-z\d+.-]*:\/\/[^/?#]*/i, "");
  return { key: link, title: rest === "" ? "/" : rest, url: link };
}

function refusalText(refusal: LinkRefusal): string {
  const label = pluginLabel(refusal.plugin);
  const domains = refusal.plugin.domains;
  return domains.length === 0
    ? text.noLinkFetch(label)
    : text.linkUnsupported(label, refusal.host, domains);
}

/** The person's other names, one press from a search, since a site may list the person under any. */
function OtherNames({ flow, person }: { flow: PersonFetchFlow; person: Person }) {
  const names = [person.name, ...person.aliases.map((alias) => alias.name)].filter(
    (name, index, all) => name !== flow.query.trim() && all.indexOf(name) === index,
  );
  if (names.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-1">
      <span className="text-xs text-muted-foreground">{text.otherNames}</span>
      {names.map((name) => (
        <Button
          key={name}
          variant="secondary"
          size="sm"
          className="max-w-full"
          disabled={flow.running !== null}
          onClick={() => flow.searchFor(name)}
        >
          <span className="max-w-chip-label truncate">{name}</span>
        </Button>
      ))}
    </div>
  );
}

function NameSearch({ flow, person }: { flow: PersonFetchFlow; person: Person }) {
  const busy = flow.running !== null;
  const candidates = flow.candidates;
  return (
    <>
      {flow.missingId && (
        <p className="text-sm text-muted-foreground">
          {text.noAcceptedId(flow.plugin === undefined ? "" : pluginLabel(flow.plugin))}
        </p>
      )}
      <QueryField
        label={text.name}
        value={flow.query}
        busy={busy}
        autoFocus
        description={isLink(flow.query) ? text.linkTyped : undefined}
        error={flow.linkRefusal === null ? undefined : refusalText(flow.linkRefusal)}
        onChange={flow.changeQuery}
      />
      <OtherNames flow={flow} person={person} />
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

/** The person's links the plugin handles, after the server found several; pressing one fetches it. */
function LinkChoice({
  flow,
  links,
}: {
  flow: PersonFetchFlow;
  links: readonly string[];
}) {
  const label = flow.plugin === undefined ? "" : pluginLabel(flow.plugin);
  return (
    <>
      <p className="text-sm text-muted-foreground">{text.chooseLink(label)}</p>
      <CandidateList
        candidates={links.map(linkView)}
        heading={text.handledLinks(label)}
        choosable
        chosenKey={flow.running?.key}
        busy={flow.running !== null}
        onChoose={(link) => flow.choose({ link })}
      />
    </>
  );
}

function SearchByName({ flow }: { flow: PersonFetchFlow }) {
  return (
    <Button
      variant="ghost"
      size="sm"
      className="self-start"
      disabled={flow.applying || flow.running !== null}
      onClick={flow.searchByName}
    >
      <Search aria-hidden="true" />
      {text.searchByName}
    </Button>
  );
}

function FindStep({
  flow,
  plugins,
  person,
}: {
  flow: PersonFetchFlow;
  plugins: readonly WorkInfoPlugin[];
  person: Person;
}) {
  return (
    <>
      <PluginField
        plugins={plugins}
        plugin={flow.plugin}
        disabled={flow.running !== null}
        onPlugin={flow.choosePlugin}
      />
      {flow.byName ? (
        <NameSearch flow={flow} person={person} />
      ) : (
        <>
          {flow.links !== null && <LinkChoice flow={flow} links={flow.links} />}
          <SearchByName flow={flow} />
        </>
      )}
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
  const label = pluginLabel(fetched.plugin);
  const link = fetched.result.link;
  return (
    <div className="flex flex-col gap-3">
      <p className="text-xs break-words text-muted-foreground">
        {link === undefined || link === ""
          ? text.source(label, first?.source ?? "", first?.value ?? "")
          : text.sourceLink(label, link)}
      </p>
      {/* Back reaches the plugin and the candidates, so another plugin or person is a step away. */}
      <div className="flex flex-wrap gap-1">
        <Button
          variant="ghost"
          size="sm"
          disabled={flow.applying || flow.running !== null}
          onClick={flow.back}
        >
          <ArrowLeft aria-hidden="true" />
          {flow.byName ? t.player.workInfoFetch.back : text.back}
        </Button>
        {!flow.byName && <SearchByName flow={flow} />}
      </div>
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
        imageSrc={(index) =>
          personPluginFetchImageUrl(person.id, fetched.result.fetchId, index)
        }
        selection={flow.selection}
        locked={flow.applying || flow.stale}
        onChange={flow.changeSelection}
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
  return flow.byName && !isLink(flow.query) ? words.search : words.fetch;
}

function submitDisabled(flow: PersonFetchFlow): boolean {
  if (flow.step === "result") return false;
  if (flow.step === "compare")
    return selectionEmpty(flow.selection) || flow.stale || flow.running !== null;
  if (flow.plugin === undefined || flow.running !== null) return true;
  return flow.byName ? flow.query.trim() === "" : flow.links !== null;
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
      {flow.step === "find" && <FindStep flow={flow} plugins={plugins} person={person} />}
      {flow.step === "compare" && <CompareStep flow={flow} person={person} />}
      {flow.step === "result" && <ResultStep lines={flow.skipped} />}
    </FormDialog>
  );
}

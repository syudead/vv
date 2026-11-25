import { useCallback, useEffect, useState } from "react";

import { isAborted } from "../../api/client";
import {
  isCreditGone,
  listOrganizationChildren,
  type Organization,
} from "../../api/credits";
import { errorText, t, type UiText } from "../../i18n";
import { FoldedEntries } from "../../player/WorkInfo";
import { type Fact, FactList } from "../../ui/patterns/fact-list";
import { Shelf } from "../../ui/patterns/shelf";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { OrganizationLink } from "./OrganizationLink";

const text = t.credits.organizationPage;

/** Children past this many fold behind `+N more`. */
const childrenShown = 12;

type ChildrenState =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "ready"; items: readonly Organization[] };

/** The organization's direct children, read once; a missing organization calls onGone. */
function useChildren(id: number, onGone: () => void) {
  const [state, setState] = useState<ChildrenState>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setState({ kind: "loading" });
    listOrganizationChildren(id, controller.signal).then(
      (list) => {
        if (!controller.signal.aborted) setState({ kind: "ready", items: list.items });
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        if (isCreditGone(failure)) {
          onGone();
          return;
        }
        setState({ kind: "failed", reason: errorText(failure) });
      },
    );
    return () => controller.abort();
  }, [id, attempt, onGone]);
  const retry = useCallback(() => setAttempt((value) => value + 1), []);
  return { state, retry };
}

function childrenValue(state: ChildrenState, retry: () => void) {
  if (state.kind === "loading") return <Skeleton className="h-4 w-1/2" />;
  if (state.kind === "failed") {
    return (
      <span className="flex flex-wrap items-center gap-x-2 text-destructive">
        {state.reason}
        <Button variant="link" size="sm" className="h-auto p-0" onClick={retry}>
          {t.common.retry}
        </Button>
      </span>
    );
  }
  return (
    <FoldedEntries
      label={text.children}
      limit={childrenShown}
      entries={state.items.map((child) => ({
        key: String(child.id),
        node: <OrganizationLink organization={child} />,
      }))}
    />
  );
}

/**
 * `Related organizations`: the parent and the children as links, one level each way. A row without
 * a value is left out, and with neither the shelf is.
 */
export function OrganizationFamily({
  organization,
  onGone,
}: {
  organization: Organization;
  onGone: () => void;
}) {
  const { state, retry } = useChildren(organization.id, onGone);
  const facts: Fact[] = [];
  if (organization.parent !== undefined) {
    facts.push({
      id: "parent",
      term: text.parent,
      value: <OrganizationLink organization={organization.parent} />,
    });
  }
  if (state.kind !== "ready" || state.items.length > 0) {
    facts.push({
      id: "children",
      term: text.children,
      value: childrenValue(state, retry),
    });
  }
  if (facts.length === 0) return null;
  return (
    <Shelf title={text.family}>
      <FactList facts={facts} />
    </Shelf>
  );
}

import { useMemo, useState } from "react";

import type { components } from "../api/gen/openapi";
import { useVideos, type VideosState } from "../api/useVideos";
import { t } from "../i18n";
import { Shelf } from "../ui/patterns/shelf";
import type { ListCriteria } from "../videoList/listCriteria";
import { ShelfToolbar } from "../videoList/ShelfToolbar";
import { type ShelfCriteria, useShelfCriteria } from "../videoList/useShelfCriteria";
import {
  ShelfFailed,
  ShelfLoading,
  ShelfVideos,
  videoShelfStatus,
} from "../videoList/VideoShelf";

// One shelf per role for the person page and the organization page; see docs/design-docs/people.md.

type PersonRole = components["schemas"]["WorkPersonRole"];
type OrganizationRole = components["schemas"]["WorkOrganizationRole"];

/** Whose videos the shelves show, and the roles in their order with each shelf's heading. */
export type ShelfSubject =
  | {
      kind: "person";
      id: number;
      roles: readonly { role: PersonRole; title: string }[];
    }
  | {
      kind: "organization";
      id: number;
      roles: readonly { role: OrganizationRole; title: string }[];
    };

const text = t.credits.personPage;

/** The subject's videos under the page's search, filter and sort, in one role or in any. */
function useSubjectVideos(
  kind: ShelfSubject["kind"],
  id: number,
  list: ListCriteria,
  role?: string,
): VideosState {
  const { query, favorite, sort, seed } = list;
  const criteria = useMemo(() => {
    const shared = { query, favorite, sort, seed };
    return kind === "person"
      ? { ...shared, person: id, personRole: role as PersonRole | undefined }
      : {
          ...shared,
          organization: id,
          organizationRole: role as OrganizationRole | undefined,
        };
  }, [kind, id, role, query, favorite, sort, seed]);
  return useVideos(criteria, undefined, "videos");
}

/**
 * The page's search, filter and sort in the top bar, then one shelf per role the subject has
 * matching videos in, in the roles' order. The matching videos in any role stand for the shelves
 * while loading or failing, and with none the empty or no-match state shows instead. With
 * `keepFailedRole` a role whose first read fails keeps its shelf with the failure.
 */
export function CreditShelves({
  subject,
  keepFailedRole = false,
}: {
  subject: ShelfSubject;
  keepFailedRole?: boolean;
}) {
  const shelf = useShelfCriteria();
  return (
    <>
      <ShelfToolbar {...shelf.toolbar} />
      <SubjectShelves subject={subject} shelf={shelf} keepFailedRole={keepFailedRole} />
    </>
  );
}

function SubjectShelves({
  subject,
  shelf,
  keepFailedRole,
}: {
  subject: ShelfSubject;
  shelf: ShelfCriteria;
  keepFailedRole: boolean;
}) {
  const all = useSubjectVideos(subject.kind, subject.id, shelf.criteria);
  const status = videoShelfStatus({
    videos: all,
    filtered: shelf.filtered,
    emptyTitle: text.noVideos,
    onSearch: () => shelf.toolbar.searchRef.current?.focus(),
  });
  if (status !== null) return status;
  return (
    <>
      {subject.roles.map(({ role, title }) => (
        <RoleShelf
          key={role}
          kind={subject.kind}
          id={subject.id}
          list={shelf.criteria}
          role={role}
          title={title}
          keepFailed={keepFailedRole}
        />
      ))}
    </>
  );
}

/**
 * Every matching video in one role. A role without matching videos draws nothing; one whose first
 * read failed draws nothing too, unless `keepFailed`, which also keeps the heading while that
 * failed read is retried.
 */
function RoleShelf({
  kind,
  id,
  list,
  role,
  title,
  keepFailed,
}: {
  kind: ShelfSubject["kind"];
  id: number;
  list: ListCriteria;
  role: string;
  title: string;
  keepFailed: boolean;
}) {
  const videos = useSubjectVideos(kind, id, list, role);
  const { items, loading, error } = videos;
  const [retrying, setRetrying] = useState(false);
  if (loading) {
    if (!retrying) return null;
    return (
      <Shelf title={title}>
        <ShelfLoading />
      </Shelf>
    );
  }
  if (items.length === 0) {
    if (!keepFailed || error === null) return null;
    const retry = () => {
      setRetrying(true);
      videos.reload();
    };
    return (
      <Shelf title={title}>
        <ShelfFailed error={error} onRetry={retry} />
      </Shelf>
    );
  }
  return (
    <Shelf title={title} count={t.count.videos(videos.total)}>
      <ShelfVideos videos={videos} />
    </Shelf>
  );
}

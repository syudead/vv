import { useEffect, useState } from "react";

import type { AssistantDestination, AssistantLabelKind } from "../api/assistant";
import { getGroup, getVideo } from "../api/client";
import { getCredit } from "../api/credits";
import { getPlaylist } from "../api/playlists";
import { listSmartFolders } from "../api/smartFolders";
import { revalidateTags } from "../api/tags";
import { creditDisplayName } from "../credits/displayName";
import { t, type UiText } from "../i18n";

// Outcomes name a destination by kind and id only, so the thread reads the name the page shows.

async function entityName(
  kind: AssistantDestination["kind"],
  id: number,
  signal: AbortSignal,
): Promise<string | undefined> {
  switch (kind) {
    case "video":
      return (await getVideo(id, signal)).title;
    case "person":
    case "organization":
      return creditDisplayName(await getCredit(kind, id, signal));
    case "group":
      return (await getGroup(id, signal)).group.name;
    case "playlist":
      return (await getPlaylist(id, signal)).name;
    case "smartFolder":
      return (await listSmartFolders(signal)).find((folder) => folder.id === id)?.name;
    case "tag":
      return (await revalidateTags()).find((tag) => tag.id === id)?.name;
    default:
      return undefined;
  }
}

function isLabelKind(kind: string): kind is AssistantLabelKind {
  return Object.hasOwn(t.assistant.clarify.kinds, kind);
}

/** The names of the pages that show no entity, as the navigation names them. */
function pageName(destination: AssistantDestination): UiText | undefined {
  const nav = t.shell.nav;
  switch (destination.kind) {
    case "settings":
      return destination.settingsPage === undefined
        ? t.settings.title
        : t.settings.pages[destination.settingsPage];
    case "library":
      return nav.library;
    case "folders":
      return nav.folders;
    case "tags":
      return nav.tags;
    case "people":
      return nav.people;
    case "organizations":
      return nav.organizations;
    case "playlists":
      return nav.playlists;
    case "history":
      return nav.history;
    case "duplicates":
      return nav.duplicates;
    case "activity":
      return nav.activity;
    default:
      return undefined;
  }
}

const routeNames: Readonly<Record<string, () => string>> = {
  "/": () => t.shell.nav.library,
  "/history": () => t.shell.nav.history,
  "/welcome": () => t.onboarding.documentTitle,
};

/**
 * The title of the page an excluded operation is done on, from its route; undefined for a route
 * with a parameter, such as a video page, which names no page to open.
 */
export function routePageName(route: string): string | undefined {
  const slug = route.startsWith("/settings/") ? route.slice("/settings/".length) : undefined;
  if (slug !== undefined) {
    const pages: Readonly<Record<string, string>> = t.settings.pages;
    const page = pages[slug];
    return page === undefined ? undefined : t.assistant.settingsPage(page);
  }
  return routeNames[route]?.();
}

/** Undefined while an entity's name loads, or when it cannot be read. */
export function useDestinationName(
  destination: AssistantDestination | undefined,
): string | undefined {
  const page = destination === undefined ? undefined : pageName(destination);
  const kind = destination?.kind;
  const id = destination?.id;
  const [loaded, setLoaded] = useState<{ key: string; name: string }>();
  const key = `${String(kind)}:${String(id)}`;
  useEffect(() => {
    if (page !== undefined || kind === undefined || id === undefined) return;
    const controller = new AbortController();
    // A name that cannot be read falls back to the kind, so the line never waits forever.
    const fallback = isLabelKind(kind) ? t.assistant.clarify.kinds[kind] : "";
    entityName(kind, id, controller.signal).then(
      (name) => {
        if (!controller.signal.aborted) setLoaded({ key, name: name ?? fallback });
      },
      () => {
        if (!controller.signal.aborted) setLoaded({ key, name: fallback });
      },
    );
    return () => controller.abort();
  }, [page, kind, id, key]);
  if (page !== undefined) return page;
  return loaded?.key === key ? loaded.name : undefined;
}

import type {
  AssistantPin,
  AssistantRoute,
  AssistantTurnContext,
} from "../api/assistant";
import type { SmartRule } from "../api/smartFolders";
import { t, type UiText } from "../i18n";
import { parseCreditFilter } from "../library/creditCriteria";
import { type LibrarySelection, selectedCount } from "../library/selection";
import { LIBRARY_PATH } from "../library/libraryPath";
import { parseTagParam, TAG_PARAM } from "../library/tagCriteria";
import { parseVideoConditions } from "../library/videoConditions";
import { cleanRule, hasRuleMember, ruleFromLibrary } from "../smartFolders/smartRule";
import { parseListCriteria } from "../videoList/listCriteria";

/** What a page tells the assistant about itself: its selection and the name it shows. */
export interface AssistantScreen {
  /** Selected video ids, the members of selected groups included. */
  videoIds?: readonly number[];
  groupIds?: readonly number[];
  smartFolderIds?: readonly number[];
  /** Selected items as the page's selection bar counts them. */
  count?: number;
  /** The open video's title, or the group, smart folder, playlist or person the page shows. */
  name?: string;
}

/** The screen of a page whose list selects videos, groups and smart folders. */
export function selectionScreen(
  selection: LibrarySelection,
  name?: string,
): AssistantScreen {
  return {
    videoIds: [...selection.ids],
    groupIds: [...selection.groups.keys()],
    smartFolderIds: [...selection.smartFolders],
    count: selectedCount(selection),
    ...(name !== undefined && { name }),
  };
}

const entityRoutes: readonly [RegExp, AssistantRoute["kind"]][] = [
  [/^\/videos\/(\d+)(?:\/|$)/, "video"],
  [/^\/people\/(\d+)(?:\/|$)/, "person"],
  [/^\/playlists\/(\d+)(?:\/|$)/, "playlist"],
  [/^\/groups\/(\d+)(?:\/|$)/, "group"],
  [/^\/smart-folders\/(\d+)(?:\/|$)/, "smartFolder"],
];

const listRoutes: readonly [RegExp, AssistantRoute["kind"]][] = [
  [/^\/folders(?:\/|$)/, "folder"],
  [/^\/people\/?$/, "people"],
  [/^\/tags\/?$/, "tags"],
  [/^\/duplicates\/?$/, "duplicates"],
  [/^\/settings(?:\/|$)/, "settings"],
];

/** The route kind and id the server resolves これ and ここ against. */
export function routeOf(pathname: string): AssistantRoute {
  if (pathname === LIBRARY_PATH) return { kind: "library" };
  for (const [pattern, kind] of entityRoutes) {
    const match = pattern.exec(pathname);
    const id = Number(match?.[1]);
    if (match !== null && Number.isSafeInteger(id) && id > 0) return { kind, id };
  }
  const list = listRoutes.find(([pattern]) => pattern.test(pathname));
  return { kind: list?.[1] ?? "other" };
}

/** The library's applied filter from its URL, or undefined when nothing filters it. */
export function libraryRule(search: string): SmartRule | undefined {
  const params = new URLSearchParams(search);
  const rule = ruleFromLibrary({
    criteria: parseListCriteria(params).criteria,
    tagIds: parseTagParam(params.getAll(TAG_PARAM)),
    credits: parseCreditFilter(params),
    conditions: parseVideoConditions(params),
  });
  return hasRuleMember(rule) ? cleanRule(rule) : undefined;
}

export interface TurnLocation {
  pathname: string;
  search: string;
}

/** The context of one turn, from the route, the published selection and the clarification pins. */
export function turnContext(
  location: TurnLocation,
  screen: AssistantScreen,
  pinned: readonly AssistantPin[],
): AssistantTurnContext {
  const route = routeOf(location.pathname);
  const context: AssistantTurnContext = { route };
  if (screen.videoIds !== undefined && screen.videoIds.length > 0) {
    context.selectedVideoIds = [...screen.videoIds];
  }
  const groups = screen.groupIds ?? [];
  const smartFolders = screen.smartFolderIds ?? [];
  if (groups.length > 0 || smartFolders.length > 0) {
    context.selectedCollectionIds = {
      ...(groups.length > 0 && { groups: [...groups] }),
      ...(smartFolders.length > 0 && { smartFolders: [...smartFolders] }),
    };
  }
  if (route.kind === "library") {
    const filter = libraryRule(location.search);
    if (filter !== undefined) context.filter = filter;
    const parsed = parseListCriteria(new URLSearchParams(location.search));
    if (parsed.hasExplicitSort) context.sort = parsed.criteria.sort;
  }
  if (pinned.length > 0) context.pinned = [...pinned];
  return context;
}

export function hasSelection(screen: AssistantScreen): boolean {
  return (
    (screen.videoIds?.length ?? 0) > 0 ||
    (screen.groupIds?.length ?? 0) > 0 ||
    (screen.smartFolderIds?.length ?? 0) > 0
  );
}

const namedRoutes: ReadonlySet<AssistantRoute["kind"]> = new Set([
  "person",
  "playlist",
  "group",
  "smartFolder",
]);

/** The line above the field that says what これ, 選んだもの and ここ will mean. */
export function contextLine(location: TurnLocation, screen: AssistantScreen): UiText {
  const words = t.assistant.context;
  const collections =
    (screen.groupIds?.length ?? 0) + (screen.smartFolderIds?.length ?? 0);
  if (hasSelection(screen)) {
    const count =
      screen.count ??
      (screen.videoIds?.length ?? 0) + (screen.smartFolderIds?.length ?? 0);
    return collections > 0 ? words.selectedItems(count) : words.selectedVideos(count);
  }
  const route = routeOf(location.pathname);
  if (route.kind === "library" && libraryRule(location.search) !== undefined) {
    return words.filteredLibrary;
  }
  if (screen.name !== undefined && screen.name !== "") {
    if (route.kind === "video") return words.video(screen.name);
    if (namedRoutes.has(route.kind)) return words.named(screen.name);
  }
  return words.screen;
}

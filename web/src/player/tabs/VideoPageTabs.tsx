import { type ReactNode, type RefObject, useEffect, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router";

import type { Video } from "../../api/client";
import type { DetailMark } from "../../api/useVideoDetail";
import { formatNumber, t, type UiText } from "../../i18n";
import { isNarrowVideo } from "../../lib/format";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../../ui/shadcn/tabs";
import DetailsEntry, { type DetailsEntryProps } from "../detailsEditor/DetailsEntry";
import type { ThumbnailCapture } from "../playerLayers";
import { useThumbnailUpload } from "../thumbnailUpload/UploadEntry";
import UploadPreview from "../thumbnailUpload/UploadPreview";
import VisibilitySwitch from "../VisibilitySwitch";
import { approvedWorkPlugins } from "../workInfoFetch/fetchRules";
import FetchDialog from "../workInfoFetch/FetchDialog";
import { useApprovedPlugins } from "../workInfoFetch/useApprovedPlugins";
import FileMediaPanel from "./FileMediaPanel";
import OverviewPanel from "./OverviewPanel";
import PeoplePanel, { type SpanPlayer } from "./PeoplePanel";
import ThumbnailPanel from "./ThumbnailPanel";
import { useThumbnailPosition } from "./useThumbnailPosition";
import VersionsPanel, { type VersionsNavigation } from "./VersionsPanel";
import {
  availableTabs,
  creditedPeopleCount,
  hasTabParam,
  nextTab,
  searchShowsTab,
  searchWithTab,
  tabFromSearch,
  type VideoTab,
} from "./videoTabs";
import WorkPanel from "./WorkPanel";

export interface VideoPageTabsProps {
  video: Video;
  owner: boolean;
  /** Supplied only for owners while the player is visible. */
  capture: ThumbnailCapture | undefined;
  /** The player's controls while it is shown, otherwise null. */
  player: SpanPlayer;
  onChanged: (video: Video, mark: DetailMark) => void;
  onStale: () => void;
  /** Suspend page shortcuts while the thumbnail preview or the fetch dialog is open. */
  onEditingChange: (open: boolean) => void;
  details: DetailsEntryProps["details"];
  state: VideoTabState;
  versions: {
    navigation: VersionsNavigation;
    /** Replace the page's video after its own version is removed. */
    onReplace: (video: Video, mark: DetailMark) => void;
  };
}

/**
 * The tab memory of the page: the work-info plugin list, read once for the owner, and the selected
 * tab, kept while each video response has it and otherwise Overview. It lives above the loading
 * state, so it survives the moment between two videos.
 *
 * The address's `tab` mirrors the selection, replaced in place, so leaving the page and coming
 * back reopens the tab. A navigation whose address has a `tab` selects it, or Overview for an
 * unknown name; one without, such as a link to another video, keeps the remembered tab and writes
 * it into the address. Only Work named by the first response's address waits for the plugin list,
 * which may add it.
 */
export function useVideoTabState(video: Video | undefined, owner: boolean) {
  const { plugins, loaded, reload } = useApprovedPlugins(approvedWorkPlugins, owner);
  const workPlugin = plugins.length > 0;
  const location = useLocation();
  const navigate = useNavigate();
  const addressTab = tabFromSearch(location.search);
  const [selected, setSelected] = useState<VideoTab>(addressTab ?? "overview");
  const [locationKey, setLocationKey] = useState(location.key);
  const [checked, setChecked] = useState<{ video?: Video; loaded: boolean }>({ loaded });
  let next = selected;
  if (locationKey !== location.key) {
    setLocationKey(location.key);
    if (hasTabParam(location.search)) next = addressTab ?? "overview";
  }
  if (video !== undefined && (checked.video !== video || checked.loaded !== loaded)) {
    setChecked({ video, loaded });
    const awaitingWork =
      next === "work" && owner && !loaded && checked.video === undefined;
    if (!awaitingWork) next = nextTab(next, availableTabs(video, owner, workPlugin));
  }
  if (next !== selected) setSelected(next);

  const { pathname, search, hash, state } = location;
  useEffect(() => {
    if (searchShowsTab(search, selected)) return;
    void navigate(
      { pathname, search: searchWithTab(search, selected), hash },
      { replace: true, state: state as unknown, preventScrollReset: true },
    );
  }, [hash, navigate, pathname, search, selected, state]);

  return { plugins, reload, workPlugin, selected, select: setSelected };
}

export type VideoTabState = ReturnType<typeof useVideoTabState>;

/**
 * Operations that outlive a tab switch: the thumbnail sends, the upload preview and the work-info
 * fetch dialog. The panels that start them receive them as props; the dialogs render here.
 */
function useOperations({
  video,
  capture,
  onChanged,
  onStale,
  onEditingChange,
}: Omit<VideoPageTabsProps, "owner" | "details" | "state" | "versions" | "player">) {
  const [failure, setFailure] = useState<UiText | null>(null);
  const [fetchOpen, setFetchOpen] = useState(false);
  const [videoId, setVideoId] = useState(video.id);
  if (videoId !== video.id) {
    setVideoId(video.id);
    setFailure(null);
    setFetchOpen(false);
  }
  const thumbnail = useThumbnailPosition(video.id, setFailure, onChanged, onStale);
  const upload = useThumbnailUpload(video, setFailure, onChanged, onStale);
  const editing = fetchOpen || upload.preview !== null;
  const editingChange = useRef(onEditingChange);
  editingChange.current = onEditingChange;
  // Page shortcuts wait while a dialog is open.
  useEffect(() => {
    if (!editing) return;
    editingChange.current(true);
    return () => editingChange.current(false);
  }, [editing]);
  return {
    failure,
    thumbnail,
    upload,
    fetchOpen,
    openFetch: () => {
      capture?.pause();
      setFetchOpen(true);
    },
    closeFetch: () => setFetchOpen(false),
  };
}

/** Keep the selected tab in view inside the strip without scrolling the page. */
function useActiveTabInView(current: VideoTab) {
  const strip = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const scroller = strip.current;
    const active = scroller?.querySelector<HTMLElement>('[data-state="active"]');
    if (scroller == null || active == null) return;
    const start = active.offsetLeft - scroller.offsetLeft;
    const end = start + active.offsetWidth;
    if (start < scroller.scrollLeft) scroller.scrollLeft = start;
    else if (end > scroller.scrollLeft + scroller.clientWidth) {
      scroller.scrollLeft = end - scroller.clientWidth;
    }
  }, [current]);
  return strip;
}

/**
 * Focus Overview, the first tab, after the render that removes the focused control; focusing a
 * tab also selects it.
 */
function focusOverview(strip: RefObject<HTMLDivElement | null>) {
  requestAnimationFrame(() => {
    strip.current?.querySelector<HTMLElement>('[role="tab"]')?.focus();
  });
}

function tabCount(tab: VideoTab, video: Video): number | undefined {
  if (tab === "people") return creditedPeopleCount(video.workInfo);
  if (tab === "versions") return video.versions?.count;
  return undefined;
}

interface PanelContext {
  workPlugin: boolean;
  operations: ReturnType<typeof useOperations>;
  strip: RefObject<HTMLDivElement | null>;
}

/** Only the selected panel is mounted, so a panel reads its data when it is opened. */
function panel(
  tab: VideoTab,
  props: VideoPageTabsProps,
  { workPlugin, operations, strip }: PanelContext,
): ReactNode {
  const { video, owner, player } = props;
  switch (tab) {
    case "overview":
      return <OverviewPanel video={video} owner={owner} />;
    case "work":
      return (
        <WorkPanel
          info={video.workInfo}
          canFetch={workPlugin}
          onFetch={operations.openFetch}
        />
      );
    case "people":
      return <PeoplePanel video={video} player={player} />;
    case "versions":
      return (
        <VersionsPanel
          video={video}
          owner={owner}
          navigation={props.versions.navigation}
          onReplace={props.versions.onReplace}
          onRefresh={props.onStale}
          onLeave={() => focusOverview(strip)}
        />
      );
    case "thumbnail":
      return (
        <ThumbnailPanel
          video={video}
          capture={props.capture}
          position={operations.thumbnail}
          upload={operations.upload}
          failure={operations.failure}
        />
      );
    case "file":
      return <FileMediaPanel video={video} owner={owner} />;
    default:
      return null;
  }
}

/** The tab strip under the tags with the record's actions at its end, and the selected panel. */
export default function VideoPageTabs(props: VideoPageTabsProps) {
  const { video, owner, details, onStale, state } = props;
  const { plugins, reload, select } = state;
  const tabs = availableTabs(video, owner, state.workPlugin);
  const current = nextTab(state.selected, tabs);
  const operations = useOperations(props);
  const strip = useActiveTabInView(current);
  const text = t.player.tabs;
  const { upload } = operations;

  return (
    <Tabs
      value={current}
      onValueChange={(next) => select(next as VideoTab)}
      className="gap-6"
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:border-b sm:border-border">
        <div
          ref={strip}
          className="min-w-0 flex-1 overflow-x-auto border-b border-border pb-1.5 scrollbar-none sm:border-b-0"
        >
          <TabsList variant="line" aria-label={text.label} className="gap-1">
            {tabs.map((tab) => (
              <TabTrigger key={tab} tab={tab} count={tabCount(tab, video)} />
            ))}
          </TabsList>
        </div>
        {owner && (
          // Above the strip below `sm`, so the tabs alone take its line.
          <div className="order-first flex shrink-0 items-center justify-end gap-1 sm:order-none sm:pb-1">
            <VisibilitySwitch
              // Key by video ID so errors do not survive navigation.
              key={`visibility:${String(video.id)}`}
              videoId={video.id}
              isPublic={video.public}
              onChanged={onStale}
            />
            <DetailsEntry video={video} owner={owner} details={details} />
          </div>
        )}
      </div>
      <TabsContent value={current}>
        {panel(current, props, {
          workPlugin: state.workPlugin,
          operations,
          strip,
        })}
      </TabsContent>
      {operations.fetchOpen && (
        <FetchDialog
          video={video}
          plugins={plugins}
          onReload={reload}
          onApplied={props.onChanged}
          onClose={operations.closeFetch}
        />
      )}
      {upload.preview !== null && (
        <UploadPreview
          videoId={video.id}
          image={upload.preview.image}
          name={upload.preview.name}
          aspect={upload.preview.aspect}
          narrow={isNarrowVideo(video)}
          applying={upload.applying}
          onApply={upload.apply}
          onCancel={upload.cancel}
        />
      )}
    </Tabs>
  );
}

function TabTrigger({ tab, count }: { tab: VideoTab; count: number | undefined }) {
  return (
    <TabsTrigger
      value={tab}
      // The underline spans the padding, as wide as the tab's hit area.
      className="flex-none group-data-[variant=line]/tabs-list:px-3"
    >
      {t.player.tabs[tab]}
      {count !== undefined && (
        <span className="text-2xs font-normal text-muted-foreground tabular-nums">
          {formatNumber(count)}
        </span>
      )}
    </TabsTrigger>
  );
}

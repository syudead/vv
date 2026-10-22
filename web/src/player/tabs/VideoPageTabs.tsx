import { type ReactNode, useEffect, useRef, useState } from "react";

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
import { availableTabs, creditedPeopleCount, nextTab, type VideoTab } from "./videoTabs";

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
}

/**
 * The tab memory of the page: the work-info plugin list, read once for the owner, and the selected
 * tab, kept while each new video response has it and otherwise Overview. It lives above the
 * loading state, so it survives the moment between two videos. The plugin list only adds Work, so
 * its arrival never changes the selection.
 */
export function useVideoTabState(video: Video | undefined, owner: boolean) {
  const { plugins, reload } = useApprovedPlugins(approvedWorkPlugins, owner);
  const workPlugin = plugins.length > 0;
  const [selected, setSelected] = useState<VideoTab>("overview");
  const [shown, setShown] = useState(video);
  if (video !== undefined && shown !== video) {
    setShown(video);
    setSelected(nextTab(selected, availableTabs(video, owner, workPlugin)));
  }
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
}: Omit<VideoPageTabsProps, "owner" | "details" | "state" | "player">) {
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

function tabCount(tab: VideoTab, video: Video): number | undefined {
  if (tab === "people") return creditedPeopleCount(video.workInfo);
  if (tab === "versions") return video.versions?.count;
  return undefined;
}

/** Only the selected panel is mounted, so a panel reads its data when it is opened. */
function panel(
  tab: VideoTab,
  {
    video,
    owner,
    player,
    capture,
  }: Pick<VideoPageTabsProps, "video" | "owner" | "player" | "capture">,
  operations: ReturnType<typeof useOperations>,
): ReactNode {
  switch (tab) {
    case "overview":
      return <OverviewPanel video={video} owner={owner} />;
    case "people":
      return <PeoplePanel video={video} player={player} />;
    case "thumbnail":
      return (
        <ThumbnailPanel
          video={video}
          capture={capture}
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
      className="gap-0"
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:border-b sm:border-border">
        <div
          ref={strip}
          className="min-w-0 flex-1 overflow-x-auto border-b border-border pb-1.5 scrollbar-none sm:border-b-0"
        >
          <TabsList variant="line" aria-label={text.label}>
            {tabs.map((tab) => (
              <TabTrigger key={tab} tab={tab} count={tabCount(tab, video)} />
            ))}
          </TabsList>
        </div>
        {owner && (
          // Above the strip below `sm`, so the tabs alone take its line.
          <div className="order-first flex shrink-0 items-center justify-end gap-2 sm:order-none sm:pb-1">
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
      <TabsContent value={current} className="pt-4 text-base">
        {panel(current, props, operations)}
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
    <TabsTrigger value={tab} className="flex-none">
      {t.player.tabs[tab]}
      {count !== undefined && (
        <span className="font-normal text-muted-foreground tabular-nums">
          {formatNumber(count)}
        </span>
      )}
    </TabsTrigger>
  );
}

import { useEffect, useState } from "react";

import type { Video } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { copyText } from "../../lib/clipboard";
import { formatBytes } from "../../lib/format";
import { FactList } from "../../ui/patterns/fact-list";
import { Button } from "../../ui/shadcn/button";
import { useToast } from "../../ui/Toast";
import MediaInfoGroups from "../MediaInfoGroups";
import RescanVideoButton from "../RescanVideoButton";
import { useOpenFileRequest } from "../useOpenFile";
import { DateValue, FailureLine, PanelColumns, PanelSection } from "./panelParts";

/** Guest responses omit the location, so a guest sees Media and File only. */
export default function FileMediaPanel({
  video,
  owner,
}: {
  video: Video;
  owner: boolean;
}) {
  const [failure, setFailure] = useState<UiText | null>(null);
  useEffect(() => setFailure(null), [video.id]);
  const location = owner ? video.location : undefined;
  const text = t.player.tabs;
  const f = t.player.facts;
  return (
    <div className="flex flex-col gap-6">
      <PanelColumns
        main={
          <>
            {location !== undefined && (
              <PanelSection title={text.location}>
                <p className="font-mono text-xs leading-relaxed wrap-anywhere text-foreground">
                  {location.path}
                </p>
                <LocationActions
                  videoId={video.id}
                  path={location.path}
                  openable={location.openable}
                  report={setFailure}
                />
              </PanelSection>
            )}
            <PanelSection title={text.media}>
              <MediaInfoGroups video={video} />
            </PanelSection>
          </>
        }
        side={
          <FactList
            variant="divided"
            aria-label={text.fileSection}
            facts={[
              { id: "size", term: f.size, value: formatBytes(video.sizeBytes) },
              {
                id: "added",
                term: f.added,
                value: <DateValue label={f.added} value={video.addedAt} />,
              },
              {
                id: "edited",
                term: f.edited,
                value: <DateValue label={f.edited} value={video.updatedAt} />,
              },
              {
                id: "created",
                term: text.fileCreated,
                value: <DateValue label={text.fileCreated} value={video.fileCreatedAt} />,
              },
            ]}
          />
        }
      />
      <FailureLine failure={failure} />
    </div>
  );
}

function LocationActions({
  videoId,
  path,
  openable,
  report,
}: {
  videoId: number;
  path: string;
  openable: boolean;
  report: (failure: UiText | null) => void;
}) {
  const toast = useToast();
  const open = useOpenFileRequest(videoId, report);
  const f = t.player.facts;
  const copyPath = () => {
    report(null);
    copyText(path, {
      onCopied: () => toast(f.pathCopied),
      onFailed: () => toast(f.copyFailed),
    });
  };
  return (
    <div className="flex flex-wrap gap-1.5">
      {openable && (
        <Button variant="secondary" size="sm" shape="pill" onClick={open}>
          {f.openFile}
        </Button>
      )}
      <Button variant="secondary" size="sm" shape="pill" onClick={copyPath}>
        {f.copyPath}
      </Button>
      <RescanVideoButton videoId={videoId} report={report} />
    </div>
  );
}

import { Copy, ExternalLink } from "lucide-react";
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
import { DateValue, FailureLine, PanelSection } from "./panelParts";

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
      {location !== undefined && (
        <PanelSection title={text.location}>
          <p className="font-mono text-sm wrap-anywhere">{location.path}</p>
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
      <PanelSection title={text.fileSection}>
        <FactList
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
              term: f.created,
              value: <DateValue label={f.created} value={video.fileCreatedAt} />,
            },
          ]}
        />
      </PanelSection>
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
    <div className="flex flex-wrap gap-2">
      {openable && (
        <Button variant="outline" size="sm" onClick={open}>
          <ExternalLink aria-hidden="true" />
          {f.openFile}
        </Button>
      )}
      <Button variant="outline" size="sm" onClick={copyPath}>
        <Copy aria-hidden="true" />
        {f.copyPath}
      </Button>
      <RescanVideoButton videoId={videoId} report={report} />
    </div>
  );
}

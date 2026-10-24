import { ExternalLink } from "lucide-react";

import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { withoutScheme } from "../WorkInfo";

/** Three rows fit a 16:9 frame at 360px, so a fourth link turns the third row into the rest's menu. */
const maxRows = 3;

/**
 * Where to watch a virtual video, as the media frame's main content: one button per link on the
 * `overlay` scrim, which keeps them readable over any image, each opening in a new tab.
 */
export default function VirtualLinks({ urls }: { urls: readonly string[] }) {
  const shown = urls.length <= maxRows ? urls : urls.slice(0, maxRows - 1);
  const rest = urls.slice(shown.length);
  return (
    <div className="absolute inset-0 flex items-center justify-center p-4">
      <div className="flex w-full max-w-sm min-w-0 flex-col gap-2 rounded-lg bg-overlay p-3">
        <ul aria-label={t.player.virtual.links} className="flex flex-col gap-2">
          {shown.map((url, index) => (
            <li key={url} className="min-w-0">
              <Button
                asChild
                variant={index === 0 ? "default" : "secondary"}
                className="w-full"
              >
                <a href={url} title={url} target="_blank" rel="noreferrer">
                  <ExternalLink aria-hidden="true" />
                  <span className="min-w-0 truncate">{withoutScheme(url)}</span>
                </a>
              </Button>
            </li>
          ))}
        </ul>
        {rest.length > 0 && <MoreLinks urls={rest} />}
      </div>
    </div>
  );
}

function MoreLinks({ urls }: { urls: readonly string[] }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="secondary" className="w-full">
          {t.player.virtual.moreLinks(urls.length)}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="center">
        {urls.map((url) => (
          <DropdownMenuItem key={url} asChild>
            <a href={url} title={url} target="_blank" rel="noreferrer">
              <ExternalLink aria-hidden="true" />
              <span className="max-w-3xs min-w-0 truncate">{withoutScheme(url)}</span>
            </a>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

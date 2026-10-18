import type { TagKindRef } from "../api/tagKinds";
import { t } from "../i18n";
import { Badge } from "../ui/shadcn/badge";

export default function TagKindLabel({ kind }: { kind: TagKindRef | null }) {
  if (kind === null) return null;
  return (
    <Badge
      variant="outline"
      title={t.tags.kindFilter.chip(kind.name)}
      // Place above the link's expanded hit area so the label still receives title hover.
      className="relative h-5 max-w-1/2 font-normal text-muted-foreground sm:max-w-chip-label"
    >
      <span className="sr-only">{t.tags.kindLabel.prefix} </span>
      <span className="min-w-0 truncate">{kind.name}</span>
    </Badge>
  );
}

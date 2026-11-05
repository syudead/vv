import { CalendarPlus, CircleDotDashed, Clock, Eye, EyeOff } from "lucide-react";

import type { VideoConditionKind, VideoConditions } from "./videoConditions";

/** The condition chips' icons; the watch chip's follows its state. */
export function conditionIcon(kind: VideoConditionKind, conditions: VideoConditions) {
  if (kind === "duration") return <Clock aria-hidden="true" />;
  if (kind === "added") return <CalendarPlus aria-hidden="true" />;
  if (conditions.watchState === "unwatched") return <Eye aria-hidden="true" />;
  if (conditions.watchState === "inProgress")
    return <CircleDotDashed aria-hidden="true" />;
  return <EyeOff aria-hidden="true" />;
}

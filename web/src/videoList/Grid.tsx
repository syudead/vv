import type { ReactNode } from "react";

import type { Zoom } from "../preferences/viewPreferences";
import { CardGrid } from "../ui/patterns/card-grid";

export function Grid({ zoom, children }: { zoom: Zoom; children: ReactNode }) {
  return <CardGrid size={zoom}>{children}</CardGrid>;
}

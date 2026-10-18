import { AlertCircle } from "lucide-react";
import { type ComponentType, useEffect, useState } from "react";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { EmptyState } from "../videoList/states";

type PageModule = { default: ComponentType };

/** Match paths case-insensitively, like React Router. */
export function matchesPath(pathname: string, path: string) {
  const normalized = pathname.toLowerCase();
  const target = path.toLowerCase();
  return normalized === target || normalized.startsWith(`${target}/`);
}

/** Match only this path, ignoring trailing slashes, to avoid loading a parent page for child routes. */
export function matchesExactPath(pathname: string, path: string) {
  return pathname.toLowerCase().replace(/\/$/, "") === path.toLowerCase();
}

export function matchesBelowPath(pathname: string, path: string) {
  return matchesPath(pathname, path) && !matchesExactPath(pathname, path);
}

export function RouteLoadFailed({ onReload }: { onReload: () => void }) {
  return (
    <EmptyState
      icon={AlertCircle}
      tone="danger"
      title={t.app.routeLoadFailed.title}
      description={t.app.routeLoadFailed.description}
      action={
        <Button size="sm" onClick={onReload}>
          {t.common.reload}
        </Button>
      }
    />
  );
}

/**
 * Load only the requested route; preload direct entries so content and keyboard actions appear
 * together without Suspense delay. Use a predicate to distinguish routes sharing a prefix;
 * failures offer reload.
 */
export async function deferredRoute(
  load: () => Promise<PageModule>,
  path: string | ((pathname: string) => boolean),
  reload: () => void = () => window.location.reload(),
): Promise<ComponentType> {
  let loaded: ComponentType | undefined;
  const loadPage = async () => {
    loaded = (await load()).default;
    return loaded;
  };
  const pathname = window.location.pathname;
  if (typeof path === "string" ? matchesPath(pathname, path) : path(pathname)) {
    // Retry at render time before displaying a load failure.
    await loadPage().catch(() => undefined);
  }

  return function DeferredRoute() {
    const [Page, setPage] = useState(() => loaded);
    const [failed, setFailed] = useState(false);
    useEffect(() => {
      if (Page !== undefined) return;
      let live = true;
      loadPage().then(
        (page) => {
          if (live) setPage(() => page);
        },
        () => {
          if (live) setFailed(true);
        },
      );
      return () => {
        live = false;
      };
    }, [Page]);

    if (Page !== undefined) return <Page />;
    if (failed) return <RouteLoadFailed onReload={reload} />;
    return null;
  };
}

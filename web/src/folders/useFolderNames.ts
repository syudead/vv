import type { FolderListing, FolderRef } from "../api/client";
import { type FolderListingState, useRootFolderName } from "../api/useFolderListing";
import { rootFolderName } from "./folderPath";

/** The open folder's summary, child folders and the names its header and crumbs show. */
export function useFolderNames(
  folder: FolderRef,
  listing: FolderListingState<FolderListing>,
) {
  const summary = listing.data?.folder;
  const children = listing.data?.folders ?? [];
  // Guest child-folder responses omit the registered root's name because they have no absolute path.
  // Fetch the registered root separately for its display name.
  const knownFromSummary =
    summary !== undefined && (summary.rootPath !== undefined || folder.path === "");
  const rootLookup = useRootFolderName(
    folder.rootId,
    summary !== undefined && !knownFromSummary,
  );
  const rootName =
    summary === undefined
      ? undefined
      : knownFromSummary
        ? rootFolderName(summary)
        : rootLookup.status === "ready"
          ? rootLookup.name
          : undefined;
  // Omit an unavailable root name instead of leaving a permanent skeleton.
  const rootNameFailed = !knownFromSummary && rootLookup.status === "failed";
  const name =
    summary?.name ?? (folder.path === "" ? rootName : folder.path.split("/").at(-1));
  return { summary, children, rootName, rootNameFailed, name };
}

import type { Ref } from "react";

import type { FolderRef } from "../api/client";
import { t } from "../i18n";
import { isRemoteId } from "../lib/remoteId";
import { PageHeader } from "../ui/patterns/page-header";
import { RemoteCountLine } from "../videoList/RemoteItemMark";
import Breadcrumbs from "./Breadcrumbs";
import { breadcrumbsFor } from "./folderPath";

export default function FolderHeader({
  folder,
  rootName,
  listingNotFound,
  videosNotFound,
  listingFailed,
  rootNameFailed,
  searching,
  name,
  heading,
}: {
  folder: FolderRef;
  rootName: string | undefined;
  listingNotFound: boolean;
  videosNotFound: boolean;
  listingFailed: boolean;
  rootNameFailed: boolean;
  searching: boolean;
  name: string | undefined;
  heading: Ref<HTMLHeadingElement>;
}) {
  return (
    <>
      <Breadcrumbs
        crumbs={
          // Omit the root crumb when a missing folder leaves its name unknown.
          listingNotFound || videosNotFound || listingFailed || rootNameFailed
            ? breadcrumbsFor(folder, undefined).filter((crumb) => crumb !== undefined)
            : breadcrumbsFor(folder, rootName)
        }
        suffix={searching && !videosNotFound ? t.folders.searchingInside : undefined}
      />
      {/* Breadcrumbs show the location visually; keep the title screen-reader-only. */}
      <PageHeader
        titleHidden
        titleRef={heading}
        title={name ?? t.folders.title}
        count={isRemoteId(folder.rootId) ? <RemoteCountLine remote /> : undefined}
      />
    </>
  );
}

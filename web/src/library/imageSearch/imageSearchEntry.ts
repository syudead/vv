import { useCallback, useState } from "react";
import { useLocation, useNavigate } from "react-router";

import type { UiText } from "../../i18n";
import { returnEntry } from "../../player/pageDecisions";
import { LIBRARY_PATH } from "../libraryPath";
import { type GivenFrom, handOver, refuseFiles } from "./imageSearchState";

export const IMAGE_SEARCH_PATH = "/image-search";

/**
 * The history state of an `/image-search` entry: the token its search is kept under, and whether the
 * library entry it was pushed from is the one under it. Neither the file nor the answer is in it.
 */
export interface ImageSearchEntryState {
  imageSearch: string;
  libraryBelow?: true;
}

function stateField(state: unknown, name: string): unknown {
  return typeof state === "object" && state !== null
    ? (state as Record<string, unknown>)[name]
    : undefined;
}

/** The token of the search an entry shows; closing the player names it as the entry to come back to. */
export function searchToken(state: unknown): string | undefined {
  const token = stateField(state, "imageSearch");
  return (
    returnEntry(state) ?? (typeof token === "string" && token !== "" ? token : undefined)
  );
}

/** Whether going one entry back reaches the library entry the search was pushed from. */
export function libraryBelow(state: unknown): boolean {
  return stateField(state, "libraryBelow") === true;
}

function entryState(token: string, below: boolean): ImageSearchEntryState {
  return below ? { imageSearch: token, libraryBelow: true } : { imageSearch: token };
}

/**
 * The in-app way back to the library: one entry back to the library it was pushed from, as browser Back
 * does, or a replace with `/` when that entry is not under this one, so it never leaves the app.
 */
export function useBackToLibrary(): () => void {
  const navigate = useNavigate();
  const { state } = useLocation();
  const below = libraryBelow(state);
  return useCallback(() => {
    if (below) void navigate(-1);
    else void navigate(LIBRARY_PATH, { replace: true });
  }, [below, navigate]);
}

/** The search field's files and its dialog, which open the screen with a search. */
export interface FileGiving {
  /** Files dropped on or pasted into the search field. */
  onFiles: (files: readonly File[]) => void;
  dialogOpen: boolean;
  setDialogOpen: (open: boolean) => void;
  /** The dialog's `onFile`: a file refused in the browser keeps the dialog open with the reason. */
  takeDialogFile: (file: File) => UiText | null;
}

/**
 * Hands given files to `/image-search`. From the library it pushes the screen over the library entry;
 * on the screen another file replaces its entry and keeps whether the library is under it.
 * `beforeLeave` runs first, so the library can keep what it restores on the way back.
 */
export function useGiveFiles(
  mode: "push" | "replace",
  beforeLeave?: () => void,
): FileGiving {
  const navigate = useNavigate();
  const { state } = useLocation();
  const below = mode === "push" || libraryBelow(state);
  const [dialogOpen, setDialogOpen] = useState(false);

  const give = useCallback(
    (files: readonly File[], from: GivenFrom) => {
      if (files.length === 0) return;
      beforeLeave?.();
      const token = handOver(files, from);
      void navigate(IMAGE_SEARCH_PATH, {
        replace: mode === "replace",
        state: entryState(token, below),
      });
    },
    [beforeLeave, below, mode, navigate],
  );
  const onFiles = useCallback((files: readonly File[]) => give(files, "field"), [give]);
  const takeDialogFile = useCallback(
    (file: File) => {
      const refusal = refuseFiles([file]);
      if (refusal !== null) return refusal.title;
      setDialogOpen(false);
      give([file], "dialog");
      return null;
    },
    [give],
  );
  return { onFiles, dialogOpen, setDialogOpen, takeDialogFile };
}

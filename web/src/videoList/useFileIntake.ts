import {
  type ClipboardEvent,
  type DragEvent as ReactDragEvent,
  useCallback,
  useEffect,
  useState,
} from "react";

/** Where a file being dragged is: not at all, somewhere over the page, or over the field. */
export type FileDrag = "none" | "page" | "field";

function carriesFiles(event: { dataTransfer: DataTransfer | null }): boolean {
  return event.dataTransfer?.types.includes("Files") ?? false;
}

export interface FileIntake {
  drag: FileDrag;
  /** Spread on the element that takes the drop. */
  dropProps: {
    onDragEnter: (event: ReactDragEvent) => void;
    onDragOver: (event: ReactDragEvent) => void;
    onDragLeave: (event: ReactDragEvent) => void;
    onDrop: (event: ReactDragEvent) => void;
  };
  /** Takes the clipboard's files and keeps text pastes as they are. */
  onPaste: (event: ClipboardEvent) => void;
}

/**
 * A field that takes a dropped or pasted file. While `onFiles` is given, a file dragged into the
 * window marks the field, and a drop anywhere else on the page is refused so the browser does not
 * replace the page with the file. See web/registry/rules/components.md, ImageUploadDialog.
 */
export function useFileIntake(
  onFiles: ((files: File[]) => void) | undefined,
): FileIntake {
  const enabled = onFiles !== undefined;
  const [overPage, setOverPage] = useState(false);
  const [overField, setOverField] = useState(false);

  useEffect(() => {
    if (!enabled) return;
    // dragenter and dragleave fire for each element crossed; the count is zero once the drag leaves the window.
    let depth = 0;
    const end = () => {
      depth = 0;
      setOverPage(false);
      setOverField(false);
    };
    const enter = (event: DragEvent) => {
      if (!carriesFiles(event)) return;
      depth += 1;
      setOverPage(true);
    };
    const leave = (event: DragEvent) => {
      if (!carriesFiles(event)) return;
      depth -= 1;
      if (depth <= 0) end();
    };
    // A target that takes files, such as the field or an upload dialog, has already accepted the drag.
    const over = (event: DragEvent) => {
      if (!carriesFiles(event) || event.defaultPrevented) return;
      event.preventDefault();
      if (event.dataTransfer !== null) event.dataTransfer.dropEffect = "none";
    };
    const drop = (event: DragEvent) => {
      if (carriesFiles(event)) event.preventDefault();
      end();
    };
    window.addEventListener("dragenter", enter);
    window.addEventListener("dragleave", leave);
    window.addEventListener("dragover", over);
    window.addEventListener("drop", drop);
    return () => {
      window.removeEventListener("dragenter", enter);
      window.removeEventListener("dragleave", leave);
      window.removeEventListener("dragover", over);
      window.removeEventListener("drop", drop);
      end();
    };
  }, [enabled]);

  const accept = useCallback(
    (event: ReactDragEvent) => {
      if (!enabled || !carriesFiles(event)) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = "copy";
      setOverField(true);
    },
    [enabled],
  );
  const dropProps = {
    onDragEnter: accept,
    onDragOver: accept,
    onDragLeave: (event: ReactDragEvent) => {
      if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
        setOverField(false);
      }
    },
    onDrop: (event: ReactDragEvent) => {
      if (onFiles === undefined || !carriesFiles(event)) return;
      event.preventDefault();
      onFiles(Array.from(event.dataTransfer.files));
    },
  };
  const onPaste = (event: ClipboardEvent) => {
    if (onFiles === undefined || event.clipboardData.files.length === 0) return;
    event.preventDefault();
    onFiles(Array.from(event.clipboardData.files));
  };

  const drag: FileDrag = !enabled
    ? "none"
    : overField
      ? "field"
      : overPage
        ? "page"
        : "none";
  return { drag, dropProps, onPaste };
}

import { ImageUp } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

// See web/registry/rules/components.md, ImageDrop.

export interface ImageIntakeOptions {
  /** Off while the owner cannot take an image, such as during an upload. */
  enabled: boolean;
  onFile: (file: File) => void;
  /** `page` steps aside while any dialog is open; `dialog` is used by the open dialog itself. */
  scope: "page" | "dialog";
}

/** Take an image pasted or dropped anywhere in the window and report whether files are dragged over it. */
export function useImageIntake({ enabled, onFile, scope }: ImageIntakeOptions): {
  dragging: boolean;
} {
  const [dragging, setDragging] = useState(false);
  const take = useRef(onFile);
  take.current = onFile;

  useEffect(() => {
    if (!enabled) return;
    // enter and leave fire for every element crossed, so count them to know when the drag leaves the window.
    let depth = 0;
    const yields = () => scope === "page" && overlayOpen();
    const reset = () => {
      depth = 0;
      setDragging(false);
    };

    const onPaste = (event: ClipboardEvent) => {
      if (event.defaultPrevented || yields() || event.clipboardData === null) return;
      // Text copied with a picture, as from a web page, belongs in the field being typed in.
      if (editable(event.target) && event.clipboardData.types.includes("text/plain"))
        return;
      const file = pickFile(event.clipboardData.files);
      if (file === null) return;
      event.preventDefault();
      take.current(file);
    };
    const onDragEnter = (event: DragEvent) => {
      if (!carriesFiles(event) || yields()) return;
      event.preventDefault();
      depth += 1;
      setDragging(true);
    };
    // A yielding page still cancels file drags, or a drop on a dialog that takes no image would leave the app to open the file.
    const onDragOver = (event: DragEvent) => {
      if (!carriesFiles(event)) return;
      event.preventDefault();
      if (!yields() && event.dataTransfer !== null)
        event.dataTransfer.dropEffect = "copy";
    };
    const onDragLeave = (event: DragEvent) => {
      if (!carriesFiles(event) || depth === 0) return;
      depth -= 1;
      if (depth === 0) setDragging(false);
    };
    const onDrop = (event: DragEvent) => {
      if (!carriesFiles(event)) return;
      reset();
      event.preventDefault();
      if (yields() || event.dataTransfer === null) return;
      const file = pickFile(event.dataTransfer.files);
      if (file !== null) take.current(file);
    };

    window.addEventListener("paste", onPaste);
    window.addEventListener("dragenter", onDragEnter);
    window.addEventListener("dragover", onDragOver);
    window.addEventListener("dragleave", onDragLeave);
    window.addEventListener("drop", onDrop);
    return () => {
      window.removeEventListener("paste", onPaste);
      window.removeEventListener("dragenter", onDragEnter);
      window.removeEventListener("dragover", onDragOver);
      window.removeEventListener("dragleave", onDragLeave);
      window.removeEventListener("drop", onDrop);
      setDragging(false);
    };
  }, [enabled, scope]);

  return { dragging: enabled && dragging };
}

/** The whole-window sign that dropping the dragged files will use them; it never takes the pointer. */
export function ImageDropOverlay({
  dragging,
  label,
}: {
  dragging: boolean;
  label: string;
}) {
  if (!dragging) return null;
  return createPortal(
    <div
      data-slot="image-drop-overlay"
      aria-hidden="true"
      className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-overlay p-6"
    >
      <div className="flex size-full flex-col items-center justify-center gap-3 rounded-lg border-2 border-dashed border-primary text-center text-lg font-medium text-foreground [&_svg]:size-10 [&_svg]:text-primary">
        <ImageUp />
        {label}
      </div>
    </div>,
    document.body,
  );
}

/** The first image, else the first file so that the caller can refuse it with its own reason. */
export function pickFile(files: FileList | readonly File[]): File | null {
  const list = Array.from(files);
  return list.find((file) => file.type.startsWith("image/")) ?? list[0] ?? null;
}

function carriesFiles(event: DragEvent): boolean {
  return event.dataTransfer?.types.includes("Files") ?? false;
}

function overlayOpen(): boolean {
  return (
    document.querySelector(
      '[role="dialog"], [role="alertdialog"], [data-slot="popover-content"]',
    ) !== null
  );
}

function editable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.isContentEditable ||
    target instanceof HTMLTextAreaElement ||
    (target instanceof HTMLInputElement && target.type !== "file")
  );
}

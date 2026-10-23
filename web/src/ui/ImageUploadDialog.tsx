import { ImageUp } from "lucide-react";
import {
  type ChangeEvent,
  type ClipboardEvent,
  type DragEvent,
  type ReactNode,
  useRef,
  useState,
} from "react";

import { t, type UiText } from "@/i18n";
import { cn } from "@/lib/cn";
import { Alert, AlertTitle } from "@/ui/shadcn/alert";
import { Button } from "@/ui/shadcn/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/ui/shadcn/dialog";

// See web/registry/rules/components.md, ImageUploadDialog.

export interface ImageUploadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  /** The picker's types, such as "image/png,image/jpeg,image/webp". */
  accept: string;
  /** Takes the chosen, dropped or pasted file; a returned reason is shown and the dialog stays open. */
  onFile: (file: File) => UiText | null;
}

/** One place to bring an image in: a drop zone that also opens the file picker, and a paste anywhere in the dialog. */
export function ImageUploadDialog({
  open,
  onOpenChange,
  title,
  description,
  accept,
  onFile,
}: ImageUploadDialogProps) {
  const input = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const [refusal, setRefusal] = useState<UiText | null>(null);
  const text = t.common.imageUpload;

  const take = (files: FileList | null) => {
    const file = files === null ? null : pickFile(files);
    if (file !== null) setRefusal(onFile(file));
  };
  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const { files } = event.target;
    take(files);
    // Clear the value so choosing the same file again still fires change.
    event.target.value = "";
  };
  const paste = (event: ClipboardEvent) => {
    if (event.clipboardData.files.length === 0) return;
    event.preventDefault();
    take(event.clipboardData.files);
  };
  // The whole dialog takes the drop, so a near miss of the zone never leaves the app to open the file.
  const dragOver = (event: DragEvent) => {
    if (!event.dataTransfer.types.includes("Files")) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
    setOver(true);
  };
  const drop = (event: DragEvent) => {
    if (!event.dataTransfer.types.includes("Files")) return;
    event.preventDefault();
    setOver(false);
    take(event.dataTransfer.files);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setRefusal(null);
        onOpenChange(next);
      }}
    >
      <DialogContent
        data-slot="image-upload-dialog"
        showCloseButton={false}
        onPaste={paste}
        onDragOver={dragOver}
        onDragLeave={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node | null))
            setOver(false);
        }}
        onDrop={drop}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description !== undefined && (
            <DialogDescription>{description}</DialogDescription>
          )}
        </DialogHeader>
        <button
          type="button"
          data-slot="image-upload-zone"
          data-over={over || undefined}
          onClick={() => input.current?.click()}
          className={cn(
            "flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-border px-6 py-10 text-center hover:bg-accent [&_svg]:size-8 [&_svg]:text-muted-foreground",
            over && "border-primary bg-primary-soft [&_svg]:text-primary",
          )}
        >
          <ImageUp aria-hidden="true" />
          <span className="font-medium">{text.zone}</span>
          <span className="text-sm text-muted-foreground">{text.zoneHint}</span>
        </button>
        <input
          ref={input}
          type="file"
          accept={accept}
          tabIndex={-1}
          aria-hidden="true"
          className="hidden"
          data-testid="image-upload-input"
          onChange={choose}
        />
        {refusal !== null && (
          <Alert variant="destructive">
            <AlertTitle>{refusal}</AlertTitle>
          </Alert>
        )}
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline" size="sm">
              {t.common.cancel}
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** The first image, else the first file so that the caller can refuse it with its own reason. */
export function pickFile(files: FileList | readonly File[]): File | null {
  const list = Array.from(files);
  return list.find((file) => file.type.startsWith("image/")) ?? list[0] ?? null;
}

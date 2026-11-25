import { useEffect, useState } from "react";

import { encodeReferenceURL } from "../../player/thumbnailEdit/editRules";

/** How the screen can show a given file. */
export type FileKind = "image" | "video" | "file";

export function fileKind(file: File): FileKind {
  if (file.type.startsWith("image/")) return "image";
  if (file.type.startsWith("video/")) return "video";
  return "file";
}

/** An object URL for an image or video file that lives as long as the caller shows it; null for any other file. */
export function useFilePreview(file: File, kinds: readonly FileKind[]): string | null {
  const [url, setUrl] = useState<string | null>(null);
  const shown = kinds.includes(fileKind(file));
  useEffect(() => {
    if (!shown) return;
    const objectUrl = URL.createObjectURL(file);
    setUrl(encodeReferenceURL(objectUrl));
    return () => {
      URL.revokeObjectURL(objectUrl);
      setUrl(null);
    };
  }, [file, shown]);
  return url;
}

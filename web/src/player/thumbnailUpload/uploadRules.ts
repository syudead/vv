/** Match the server's 64 MiB image limit. */
export const UPLOAD_MAX_BYTES = 64 * 1024 * 1024;

/** Match the server's pixel limit. */
export const UPLOAD_MAX_PIXELS = 120_000_000;

export const UPLOAD_TYPES = "image/png,image/jpeg,image/webp";

const uploadTypes = new Set(UPLOAD_TYPES.split(","));

/** Allow empty MIME types because the server validates content; server checks remain authoritative after client size/type validation. */
export function uploadRefusal(file: Pick<File, "type" | "size">): "type" | "size" | null {
  if (file.type !== "" && !uploadTypes.has(file.type)) return "type";
  if (file.size > UPLOAD_MAX_BYTES) return "size";
  return null;
}

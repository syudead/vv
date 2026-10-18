/**
 * Fill portrait letterboxing with a blurred copy of the image; parent requires relative and
 * overflow-hidden, foreground requires relative.
 */
/** Shared with the local-image preview canvas. */
export const thumbnailBackdropClass =
  "pointer-events-none absolute inset-0 h-full w-full scale-125 object-cover opacity-50 blur-xl";

export default function ThumbnailBackdrop({ src }: { src: string }) {
  return (
    <img
      src={src}
      alt=""
      aria-hidden="true"
      loading="lazy"
      decoding="async"
      data-thumbnail-backdrop=""
      className={thumbnailBackdropClass}
    />
  );
}

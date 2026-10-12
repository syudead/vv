import { useLayoutEffect, useRef, useState } from "react";

import { t } from "../../i18n";
import { fitBox } from "./editRules";

/**
 * CompareStage はコマと生成した画像を、動画の形で同じ大きさの箱に並べる（ui-design
 * 「Compare」）。md 未満は縦に積み、コマを上にする（並べ方は CSS が決め、ここは並んだ向きを
 * 読んで箱の大きさを測るだけ）。箱は名前と注記の分を除いた段に収まるいちばん大きな大きさで、
 * 名前は箱のすぐ上に置く。生成した画像はサムネイルと同じく箱を覆うように切り抜いて出し、
 * 形が違えば注記を箱の下に添える。
 */
export default function CompareStage({
  frameUrl,
  resultUrl,
  aspect,
  cropNote,
  onResultSize,
}: {
  frameUrl: string;
  resultUrl: string;
  aspect: number;
  cropNote: boolean;
  onResultSize: (width: number, height: number) => void;
}) {
  const text = t.player.thumbnailEdit;
  const area = useRef<HTMLDivElement | null>(null);
  const group = useRef<HTMLDivElement | null>(null);
  const label = useRef<HTMLElement | null>(null);
  const note = useRef<HTMLParagraphElement | null>(null);
  const [box, setBox] = useState({ width: 0, height: 0 });

  useLayoutEffect(() => {
    const element = area.current;
    if (element === null) return;
    const measure = () => {
      const row = group.current;
      if (row === null) return;
      const style = getComputedStyle(row);
      const gap = parseFloat(style.rowGap) || 0;
      const labelHeight = (label.current?.offsetHeight ?? 0) + 4;
      const noteHeight = note.current === null ? 0 : note.current.offsetHeight + 12;
      const width = element.clientWidth;
      const height = element.clientHeight - noteHeight;
      const next =
        style.flexDirection === "row"
          ? fitBox((width - gap) / 2, height - labelHeight, aspect)
          : fitBox(width, (height - gap) / 2 - labelHeight, aspect);
      setBox((previous) =>
        previous.width === next.width && previous.height === next.height
          ? previous
          : next,
      );
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [aspect, cropNote]);

  const size = { width: box.width, height: box.height };
  return (
    <div
      ref={area}
      className="flex size-full min-h-0 flex-col items-center justify-center gap-3"
    >
      <div ref={group} className="flex flex-col gap-4 md:flex-row">
        <figure className="flex flex-col gap-1">
          <figcaption ref={label} className="text-sm text-muted-foreground">
            {text.frame}
          </figcaption>
          <div className="overflow-hidden rounded-sm" style={size}>
            <img src={frameUrl} alt={text.frame} className="size-full object-cover" />
          </div>
        </figure>
        <figure className="flex flex-col gap-1">
          <figcaption className="text-sm text-muted-foreground">
            {text.generated}
          </figcaption>
          <div className="overflow-hidden rounded-sm" style={size}>
            <img
              src={resultUrl}
              alt={text.generated}
              onLoad={(event) =>
                onResultSize(
                  event.currentTarget.naturalWidth,
                  event.currentTarget.naturalHeight,
                )
              }
              className="size-full object-cover"
            />
          </div>
        </figure>
      </div>
      {cropNote && (
        <p ref={note} className="text-center text-sm text-muted-foreground">
          {text.cropNote}
        </p>
      )}
    </div>
  );
}

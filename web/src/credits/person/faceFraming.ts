/** Framing of a person's face in a video frame; lengths are fractions of the frame's long side, as face tracks are. */

/** Width over height of a person's face, matching the profile face's `aspect-portrait`. */
export const FACE_ASPECT = 2 / 3;

/** The smallest frame width, so a frame never shrinks to a few pixels. */
export const MIN_FACE_WIDTH = 0.05;

/** The frame holds the face this many times its width, leaving room for the hair and shoulders. */
const FACE_ROOM = 2.2;

/** The face sits this share of the frame's height above its centre, where a portrait holds it. */
const FACE_LIFT = 0.12;

export interface Size {
  width: number;
  height: number;
}

/** A portrait frame by its centre and width. */
export interface FaceFrame {
  x: number;
  y: number;
  width: number;
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

function extent(image: Size): Size {
  const long = Math.max(image.width, image.height);
  return { width: image.width / long, height: image.height / long };
}

/** The widest frame that fits the image. */
export function maxFaceWidth(image: Size): number {
  const side = extent(image);
  return Math.min(side.width, side.height * FACE_ASPECT);
}

function clamp(value: number, low: number, high: number): number {
  return Math.min(Math.max(value, low), high);
}

/** The frame kept inside the image, at least MIN_FACE_WIDTH wide unless the image is narrower. */
export function clampFaceFrame(image: Size, frame: FaceFrame): FaceFrame {
  const side = extent(image);
  const max = maxFaceWidth(image);
  const width = clamp(frame.width, Math.min(MIN_FACE_WIDTH, max), max);
  const height = width / FACE_ASPECT;
  return {
    width,
    x: clamp(frame.x, width / 2, side.width - width / 2),
    y: clamp(frame.y, height / 2, side.height - height / 2),
  };
}

/** A frame around the face at focus, or the widest centred one without a focus. */
export function initialFaceFrame(
  image: Size,
  focus: { x: number; y: number; w: number } | undefined,
): FaceFrame {
  if (focus === undefined) {
    const side = extent(image);
    return clampFaceFrame(image, {
      x: side.width / 2,
      y: side.height / 2,
      width: maxFaceWidth(image),
    });
  }
  const width = focus.w * FACE_ROOM;
  return clampFaceFrame(image, {
    x: focus.x,
    y: focus.y + (width / FACE_ASPECT) * FACE_LIFT,
    width,
  });
}

/** The frame in the image's pixels. */
export function faceFrameRect(image: Size, frame: FaceFrame): Rect {
  const long = Math.max(image.width, image.height);
  const width = frame.width * long;
  const height = width / FACE_ASPECT;
  return {
    x: frame.x * long - width / 2,
    y: frame.y * long - height / 2,
    width,
    height,
  };
}

/** The frame moved by a distance given in pixels of an image shown `shown` pixels wide. */
export function moveFaceFrame(
  image: Size,
  frame: FaceFrame,
  dx: number,
  dy: number,
  shown: number,
): FaceFrame {
  const scale = extent(image).width / shown;
  return clampFaceFrame(image, {
    ...frame,
    x: frame.x + dx * scale,
    y: frame.y + dy * scale,
  });
}

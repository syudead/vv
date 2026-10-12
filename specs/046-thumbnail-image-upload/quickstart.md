# Quickstart: Upload a real photo and edit it with a real ComfyUI

These steps prove that a photo from a real device becomes the thumbnail and
that a real model edits an uploaded photo and a generated thumbnail; `task
check` and `task test-e2e` upload generated images and run a fake ComfyUI, so
they cannot catch a camera's JPEG features or the model's output.

## Prerequisites

- vv running from the feature branch ([development.md](../../docs/how-to/development.md)),
  logged in as the owner, with one library video and a ComfyUI connection
  that passes the test in Settings ([045 quickstart](../045-comfyui-thumbnails/quickstart.md)).
- A photo taken with a phone or camera at its full resolution (a 48- or
  108-megapixel mode when it has one), a portrait photo, a PNG and a WebP of
  any size, and one image over 120 megapixels (a stitched panorama, or a
  scan).

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Video page: upload the PNG, then the WebP, then the phone photo | Each becomes the thumbnail on the library card, the video page and another video's related videos within a few seconds | 1, 2 |
| Reload; start a full scan | The photo stays | 1 |
| Upload the portrait photo to a landscape video | The card and the page show the photo's middle at the video's shape, with no bars | 3 |
| Upload the image over 120 megapixels | A failure line that says the image is too large; the thumbnail is unchanged | Edge case |
| Guest session in another browser | No upload entry, no source choice when editing | 7 |
| Start an edit from the current thumbnail; paint over one object in the photo; run; apply | The editor shows the photo as its source; the result keeps the photo outside the painted area; the thumbnail is the result | 5, 6 |
| Start an edit from the current thumbnail again | The generated image is the source, shown next to the new result after a run | 5 |
| "Use the frame instead" | The thumbnail is the frame again | 4 |
| Upload the photo again, then "use current frame as thumbnail" | The thumbnail is that frame | 4 |

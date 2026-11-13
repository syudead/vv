import { guardAudience, NetworkFailed, readJson } from "./client";

export interface UploadOptions {
  signal?: AbortSignal;
  /** Called with the share of the body sent, from 0 to 1, and with 1 once the whole body is sent. */
  onProgress: (sent: number) => void;
}

/**
 * POSTs a form and reports how much of it was sent. fetch can only do that with a streamed body, which needs
 * HTTP/2 and is missing in Firefox and Safari. The abort, the network failure, the owner check and the error
 * body behave as `request`'s do.
 */
export async function uploadRequest<T>(
  path: string,
  body: FormData,
  options: UploadOptions,
): Promise<T> {
  return readJson<T>(await guardAudience(await sendUpload(path, body, options)));
}

function aborted(): DOMException {
  return new DOMException("The upload was aborted.", "AbortError");
}

// The Response constructor refuses a body with these statuses.
const nullBodyStatuses = new Set([204, 205, 304]);

function responseHeaders(xhr: XMLHttpRequest): Headers {
  const headers = new Headers();
  for (const line of xhr
    .getAllResponseHeaders()
    .trim()
    .split(/[\r\n]+/)) {
    const colon = line.indexOf(":");
    if (colon > 0)
      headers.append(line.slice(0, colon).trim(), line.slice(colon + 1).trim());
  }
  return headers;
}

function sendUpload(
  path: string,
  body: FormData,
  { signal, onProgress }: UploadOptions,
): Promise<Response> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(aborted());
      return;
    }
    const xhr = new XMLHttpRequest();
    const abort = () => xhr.abort();
    signal?.addEventListener("abort", abort, { once: true });
    const settle = () => signal?.removeEventListener("abort", abort);
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0)
        onProgress(event.loaded / event.total);
    };
    xhr.upload.onload = () => onProgress(1);
    xhr.onload = () => {
      settle();
      const empty = nullBodyStatuses.has(xhr.status);
      resolve(
        new Response(empty ? null : (xhr.response as Blob), {
          status: xhr.status,
          statusText: xhr.statusText,
          headers: responseHeaders(xhr),
        }),
      );
    };
    xhr.onerror = () => {
      settle();
      reject(new NetworkFailed(new Error("upload failed")));
    };
    xhr.onabort = () => {
      settle();
      reject(aborted());
    };
    xhr.open("POST", path);
    xhr.responseType = "blob";
    xhr.send(body);
  });
}

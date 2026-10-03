# Technology selection: MDM (Media Data Management)

This document records the technology chosen for a system that manages video
files and plays them in a browser, and why. The current feature and data
boundaries are in [`ARCHITECTURE.md`](../../ARCHITECTURE.md).

## 1. Premises

| Item | Decision |
| --- | --- |
| Deployment | Self-hosted (one personal or home machine, Docker on a NAS or small server) |
| Client | Web browser only (PC and smartphone) |
| Video handling | Supported formats are served from the original file; unsupported formats are live-transcoded during the request |
| Users | A single account. Guests can view public videos |
| Library size | Up to tens of thousands of videos, several TB, on local disk |

## 2. Selection criteria

The system is self-hosted for a single user, so **simple operation** and
**recovery after failure** take priority over throughput.

1. **One process in one container.** Add no always-running external middleware
   (Redis, a message broker, a separate DB server).
2. **Keep the rebuildable index separate from user data.** A scan restores the
   index from the video files; it cannot restore playback positions or
   credentials ([data classification](../../ARCHITECTURE.md#rebuildable-and-user-data)).
3. **Boundaries enforced by tools.** Following the principles in
   `ARCHITECTURE.md`, lint enforces that the domain layer does not depend on
   HTTP, the DB or ffmpeg.
4. **Heavy processing inside a boundary.** Code that runs `ffmpeg` stays in an
   adapter and does not mix with HTTP or storage responsibilities.

## 3. Decisions

| Layer | Choice | Main reason |
| --- | --- | --- |
| Backend language | Go (version in `go.mod`) | Ships as a single binary, and the standard library covers a resident process and child-process management. Memory use on a NAS is small |
| HTTP server | Standard library `net/http` (the Go 1.22+ `ServeMux`) | Method routing and path wildcards are built in. `http.ServeContent` already implements Range serving correctly |
| Video delivery | `http.ServeContent` for supported formats; per-request fragmented MP4 live transcoding for unsupported formats | Range serving of the original file is left to the standard implementation, and transcoded output is not stored ([live transcoding seek](live-transcode-seek.md)) |
| Frontend | React + Vite + React Router + Tailwind CSS | An SPA whose static build is embedded in the Go binary with `embed`. React Router manages navigation |
| API contract | OpenAPI 3.1 is the source of truth; Go is generated with `oapi-codegen`, TS with `openapi-typescript` | Prevents, by tooling, the one cost a two-language setup adds: type drift |
| DB | SQLite (`modernc.org/sqlite`, no CGO, WAL mode) | Cross-compiles as a static binary and fits a small alpine-based image |
| Queries | Hand-written SQL through `database/sql` | Keeps SQL, including FTS5, as the primary source |
| Migrations | `goose` (migrations bundled in an `embed.FS`) | The binary applies them on its own with no external tool to install |
| Full-text search | SQLite FTS5 (`tokenize='trigram'`) | Substring search of Japanese without an added tokenizer. No external search engine |
| Media analysis | `ffprobe` / `ffmpeg` run through `os/exec` (timeout via `context`) | No wrapper, so arguments and failure reasons are explicit. The standard library is enough to stop processes |
| Authentication | Username and password (Argon2id from `golang.org/x/crypto/argon2`) + HttpOnly cookie session (sessions stored in SQLite) | Sufficient for a single user. Public exposure requires an HTTPS reverse proxy |
| Asynchronous work | A SQLite job table + goroutine workers (graceful stop via `context`) | No separate process or broker. Jobs resume after a restart |
| Logging | Standard library `log/slog` (JSON handler) | Structured logs with no added dependency |
| Tests | Go `testing` + `net/http/httptest` (Range checks) + Playwright (playback E2E) | Only E2E can show that playback actually starts |
| Lint | `golangci-lint` (`depguard` forbids imports across layers) | CI rejects dependency-direction violations automatically |
| Distribution | Docker (multi-stage, alpine + ffmpeg) + Compose | Without CGO the binary runs on alpine as is, and the image is small |

Dependency direction and package responsibilities are in
[ARCHITECTURE.md](../../ARCHITECTURE.md#intended-dependency-direction). SQLite
holds both the rebuildable index and user data; the
[data classification in the same document](../../ARCHITECTURE.md#rebuildable-and-user-data)
is the source of truth for telling them apart during recovery.

The content key, which identifies a video as the same video after a move or
rename, is built from the first and last 1 MiB and the file size. It identifies
a file without reading all of it and needs only the standard library's SHA-256.

## 4. Rejected alternatives

| Candidate | Reason rejected |
| --- | --- |
| TypeScript / Node.js backend | Range serving and process management would be hand-written, and native dependencies (`better-sqlite3`) need rebuild maintenance |
| Python + FastAPI | Rich libraries, but distribution is heavy and running a resident worker and managing dependencies costs too much for self-hosting |
| Echo / Gin / Fiber | The standard `ServeMux` is enough at this size, so there is no reason to add a dependency for routing. Fiber is not `net/http`-compatible and would lose the benefit of `ServeContent` |
| GORM / ent | Hand-written SQL was chosen over an ORM abstraction to keep SQL, including FTS5, as the primary source |
| `sqlc` | Not adopted, under the policy of managing SQL, including FTS5, as written |
| `mattn/go-sqlite3` | Needs CGO, which adds cross-compilation and alpine build maintenance |
| PostgreSQL | Would add a separate container and a separate backup procedure to a single-account deployment |
| Meilisearch / Elasticsearch | Better search quality, but adds a resident process. FTS5 trigram is practical for tens of thousands of items |
| SvelteKit | Light and a strong candidate. React was chosen only for its ecosystem and future staffing, not for technical superiority |
| Bulk HLS transcoding at startup | Consumes large amounts of storage and CPU; per-request live transcoding is used instead |
| Redis + an external job queue | Analysis, thumbnails and previews are processed one at a time per stage. A SQLite job table and goroutines are enough |
| S3 / MinIO | Conflicts with the premise that local disk is the source of truth, and adds a relay to Range serving |
| Adopting Jellyfin / Plex | Off-the-shelf products meet the requirements, but this repository is built from scratch by design. They serve only as references for feature comparison |

## 5. Known risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Two languages for frontend and backend | Type drift and doubled build/CI. `api/openapi.yaml` is the single source of truth for the API, and both Go and TypeScript are generated from it, so drift becomes a compile error. The build is one task in `Taskfile.yml` (`task build`: SPA build → embed → Go build) |
| FTS5 trigram tokenizer | Creation and search are verified with `modernc.org/sqlite`. Trigram `MATCH` does not match search terms of two characters or fewer, so search has two paths: `MATCH` for 3 or more characters, `instr` on the matching key `search_key` for 1–2 characters (routing in `termUsesMatch` in `internal/store/search.go`, tests in `internal/store/fts_test.go`) |
| Unicode normalization of file names | Real paths keep the spelling the file system returned. Only display names and search strings are normalized to NFC (`golang.org/x/text/unicode/norm`). Real paths are not normalized because, on Linux and similar systems, the normalized path can point at a different, non-existent entry |
| I/O saturation during a large scan | Analysis, thumbnails and previews each have a per-stage worker that processes one item at a time (concurrency 1 within a stage). Progress is counted from the `jobs` table and sent to the UI over `/api/events` |

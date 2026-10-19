# Quickstart: Face recognition with the real runtime

These steps prove what the automated checks cannot: the published runtime
downloads, starts under the Docker image and the Windows app, and links real
faces. Go and Web tests run the analysis with a fake helper.

## Prerequisites

- A build from the feature branch, run as in
  [development.md](../../docs/how-to/development.md#run-locally), once in the
  Docker image (`task up`) and once as the Windows app zip.
- Two videos in which the same actor is the main person for at least 30 s,
  and one video in which a second person appears for under 2 s. The POC's
  CelebV-HQ clips (PR #1393) serve.
- Internet access to `github.com` and `huggingface.co`.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Import the three videos with the feature off | No person is added; `<data>/face/` does not exist | 1 |
| Turn the feature on in Settings | The download shows its progress and ends `ready` | Requirement 1 |
| Choose ArcFace r50 | The screen states that its license is non-commercial | 6 |
| Choose SFace again and analyse all videos | One provisional person, linked as performer to both videos of the actor; none for the brief person | 2, 3 |
| Upload a photo of the actor to a named person | The provisional person is merged into that person, who is now the performer of both videos | 4 |
| Undo the merge from Activity | The provisional person and both links are back | 5 |
| Delete face data | People and performers stay; uploading another face merges nothing until "Analyse all" runs | 7 |
| Turn the feature off during an analysis, then on | The video is analysed again from the start | Edge case |
| Replace `<data>/face/models/*.onnx` with an empty file and restart | Settings shows `failed` with a reason and offers the download again | Edge case |

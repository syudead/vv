# Quickstart: Tag workbench at scale and the change notice

These steps check what the automated suites do not time or open in two tabs:
that the workbench stays responsive with 5,000 tags, and that a change made
elsewhere shows a notice instead of moving the list.

## Prerequisites

- A development server from [development.md](../../docs/how-to/development.md),
  signed in as the owner, with a few videos imported.
- 5,000 tags. In the browser console on the signed-in page, run:

  ```js
  for (let i = 0; i < 5000; i++) {
    await fetch("/api/tags", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: `bulk-${String(i).padStart(4, "0")}` }),
    });
  }
  ```

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Open `/tags` and scroll from the top to the end of the list | Rows keep up with the scroll and more rows load without a pause the eye notices | 4 |
| Type `bulk-49` in the search | The list narrows on each keystroke without the input lagging | 4 |
| Check 300 rows with Shift-click, then merge them into `bulk-0000` | The pane shows the selection count at once; the merge finishes and the merged rows leave the list | 4 |
| Set a filter, a sort and a search, then reload the page and press Back | The same rows show after the reload, and Back restores the previous criteria | 5 |
| Keep `/tags` open in one tab; in a second tab, rename a tag shown in the first | The first tab keeps its rows and position and shows the changed notice; refreshing shows the new name | Edge case: list changed elsewhere |
| Promote a synonym of a tag on a video, then open the library | The video's card shows the new name, and a search for the old name finds the video | 1 |

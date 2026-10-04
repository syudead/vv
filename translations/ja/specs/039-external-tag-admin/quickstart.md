---
source: specs/039-external-tag-admin/quickstart.md
sourceHash: 692d67959ea8462e1327b83c1b58ea6c7196a3f178e362d0282f27b5829bb235
---

# クイックスタート: エージェントが MCP でタグを整理する {#quickstart-an-agent-tidies-tags-through-mcp}

この手順は、実際の MCP クライアントで、`list_tags` の応答 1 つがエージェントの読める量に収まることと、ツールから行った統合がタグ管理画面に表示されるとおりであることを確かめる。`task check` は同じ操作をハンドラに対する Go のテストで実行するが、MCP クライアントは実行しない。

## 前提条件 {#prerequisites}

- API トークン ([Create a token](../../docs/how-to/external-api.md#create-a-token)) を持つ起動中のサーバーと、仮のタグが 1,000 個以上あるライブラリ。`go run ./scripts/tagsbench -scale 2000` がそのようなライブラリを用意する。用意するタグの半分が仮のタグなので、これより小さい規模では足りない ([docs/how-to/tags-admin-benchmark.md](../../docs/how-to/tags-admin-benchmark.md))。
- サーバーを登録した Claude Code: `claude mcp add --transport http vv http://localhost:8080/mcp --header "Authorization: Bearer vvt_…"`。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| 選んだ語を名前に含む仮のタグを一覧にし、残りがなくなるまで読み続けるようエージェントに頼む | エージェントが行う `list_tags` の呼び出しはどれも `tentative: true`、`q`、`cursor` を持つ。100 項目を超える応答はない。最後の応答に `nextCursor` はない。エージェントは合うタグをすべて挙げる | 1 |
| それらのタグの 1 つ (統合元) を別の 1 つ (統合先) へ、名前で指定して統合するようエージェントに頼む | エージェントは 2 つの id で `merge_tags` を呼ぶ。結果の `tag` は `synonyms` に統合元の名前を持ち、`tentative: false` であり、`notFoundIds` は `[]` である | 2 |
| タグ管理画面を開き、統合元の名前で検索する | 統合元は一覧から消えている。統合先は統合元の名前を同義語として表示する。統合元が付いていた動画には統合先が付いている | 2, 5 |
| 仮のタグを 1 つ却下し、続いて却下した名前を一覧にするようエージェントに頼む | `batch_tags` は id を `appliedIds` で返す。`list_rejected_tag_names` はそのタグの名前を返す | 3 |
| 却下した名前を `update_video_tags` で `tentative: true` を付けて付与する | その名前は `skippedTags` で返され、タグは作られない | 3 |

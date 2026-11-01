package app

import (
	"encoding/json"

	"github.com/syudead/vv/internal/domain"
)

const (
	// unknownContextPage is the page a list read gets before the provider has reported the context.
	unknownContextPage = 50
	// pageLimitMax is the largest page the list operations accept when their schema does not say.
	pageLimitMax = 200
	// itemBytesGuess is a compact list item's size before the turn has read one of that operation.
	itemBytesGuess = 400
)

// fitPageLimit sets the limit of a read whose schema takes one to the items that fit the room the prompt leaves,
// shared by the round's calls, and never above the model's own limit. A list read without a limit answered every
// match, and a page past the room was cut away.
func (l *turnLoop) fitPageLimit(tool domain.AssistantTool, raw json.RawMessage) json.RawMessage {
	maximum, ok := limitMaximum(tool.Parameters)
	if !ok {
		return raw
	}
	args := map[string]json.RawMessage{}
	if len(raw) > 0 && json.Unmarshal(raw, &args) != nil {
		return raw
	}
	limit := min(maximum, l.pageItems(tool.Operation))
	var asked int
	if json.Unmarshal(args["limit"], &asked) == nil && asked > 0 {
		limit = min(limit, asked)
	}
	args["limit"], _ = json.Marshal(max(limit, 1)) // An int always encodes.
	out, _ := json.Marshal(args)                   // Raw values from a decoded object always encode.
	return out
}

// limitMaximum reports whether the schema takes a limit, and its maximum.
func limitMaximum(parameters json.RawMessage) (int, bool) {
	var schema struct {
		Properties map[string]struct {
			Maximum *float64 `json:"maximum"`
		} `json:"properties"`
	}
	if json.Unmarshal(parameters, &schema) != nil {
		return 0, false
	}
	limit, ok := schema.Properties["limit"]
	if !ok {
		return 0, false
	}
	if limit.Maximum != nil && *limit.Maximum >= 1 {
		return int(*limit.Maximum), true
	}
	return pageLimitMax, true
}

// pageItems is how many items of the operation fit the room the instructions, the request and the tools leave,
// shared by the round's calls.
func (l *turnLoop) pageItems(operation string) int {
	budget := l.promptBudget()
	if budget <= 0 {
		return unknownContextPage
	}
	room := (budget - l.fixedTokens) / max(l.roundCalls, 1)
	size := l.itemBytes[operation]
	if size == 0 {
		size = itemBytesGuess
	}
	return room * bytesPerToken / size
}

// learnItemBytes keeps the mean size of the items a list read answered, for the next page of the operation.
func (l *turnLoop) learnItemBytes(operation string, body []byte) {
	var page struct {
		Items []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(body, &page) != nil || len(page.Items) == 0 {
		return
	}
	size := 0
	for _, item := range page.Items {
		size += len(item) + 1
	}
	if l.itemBytes == nil {
		l.itemBytes = map[string]int{}
	}
	l.itemBytes[operation] = size/len(page.Items) + 1
}

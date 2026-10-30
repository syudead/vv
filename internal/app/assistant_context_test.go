package app

import (
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Old tool results give way, oldest first, until the estimate fits; the request and the newest result stay.
func TestFitContextDropsTheOldestResultsFirst(t *testing.T) {
	big := strings.Repeat("あ", 6000) // 18,000 bytes, about 6,000 tokens.
	l := &turnLoop{usage: domain.ChatUsage{ContextTokens: 16384}, messages: []domain.ChatMessage{
		{Role: domain.ChatRoleSystem, Content: "rules"},
		{Role: domain.ChatRoleUser, Content: "<request>この動画は?</request>"},
		{Role: domain.ChatRoleAssistant},
		{Role: domain.ChatRoleTool, Content: big},
		{Role: domain.ChatRoleTool, Content: big},
		{Role: domain.ChatRoleAssistant},
		{Role: domain.ChatRoleTool, Content: big},
	}}

	l.fitContext(nil)

	got := []string{l.messages[3].Content, l.messages[4].Content, l.messages[6].Content}
	if got[0] != droppedResult || got[1] != big || got[2] != big {
		t.Fatalf("results = %.20q, want only the oldest dropped", got)
	}
	if l.messages[1].Content != "<request>この動画は?</request>" {
		t.Fatalf("request = %q, want it kept", l.messages[1].Content)
	}
}

// The latest round's results are cut, not dropped, past the budget: dropping them makes the model call the same tools
// again.
func TestFitContextCutsTheLatestRound(t *testing.T) {
	big := strings.Repeat("あ", 20000)
	l := &turnLoop{usage: domain.ChatUsage{ContextTokens: 16384}, messages: []domain.ChatMessage{
		{Role: domain.ChatRoleUser, Content: "<request>何人?</request>"},
		{Role: domain.ChatRoleAssistant},
		{Role: domain.ChatRoleTool, Content: big},
	}}

	l.fitContext(nil)

	got := l.messages[2].Content
	if !strings.HasPrefix(got, "あああ") || !strings.Contains(got, "cut to fit the context: 60000 bytes in all") ||
		len(got) >= len(big) {
		t.Fatalf("latest result = %.40q…, want its start kept and the cut noted", got)
	}
}

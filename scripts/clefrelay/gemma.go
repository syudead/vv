package main

import (
	"encoding/json"
	"strings"
)

// Gemma's own tool call format, which Workers AI sometimes leaves unparsed in reasoning_content:
// <|tool_call>call:name{key:<|"|>text<|"|>,n:1}<tool_call|>, with bare keys and strings between <|"|> marks.
const (
	gemmaCallStart = "<|tool_call>call:"
	gemmaCallEnd   = "<tool_call|>"
	gemmaQuote     = `<|"|>`
)

// gemmaCalls reads reasoning that holds only Gemma tool calls as those calls. Anything else, such as prose or
// arguments that do not become a JSON object, answers nil, so the reply stays empty.
func gemmaCalls(reasoning string) []ollamaToolCall {
	rest := strings.TrimSpace(reasoning)
	var calls []ollamaToolCall
	for rest != "" {
		body, ok := strings.CutPrefix(rest, gemmaCallStart)
		if !ok {
			return nil
		}
		call, after, ok := strings.Cut(body, gemmaCallEnd)
		if !ok {
			return nil
		}
		name, args, ok := strings.Cut(call, "{")
		object, valid := gemmaObject("{" + args)
		if !ok || name == "" || !valid {
			return nil
		}
		var wire ollamaToolCall
		wire.Function.Name, wire.Function.Arguments = name, object
		calls = append(calls, wire)
		rest = strings.TrimSpace(after)
	}
	return calls
}

// gemmaObject rewrites Gemma's arguments as JSON: each quoted text is encoded as a JSON string and each bare key is
// quoted; numbers, true, false, null and punctuation pass through.
func gemmaObject(args string) (json.RawMessage, bool) {
	var out strings.Builder
	for i := 0; i < len(args); {
		switch c := args[i]; {
		case strings.HasPrefix(args[i:], gemmaQuote):
			text, _, ok := strings.Cut(args[i+len(gemmaQuote):], gemmaQuote)
			if !ok {
				return nil, false
			}
			encoded, _ := json.Marshal(text) // A string always encodes.
			out.Write(encoded)
			i += 2*len(gemmaQuote) + len(text)
		case c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z'):
			end := i + 1
			for end < len(args) && isWordByte(args[end]) {
				end++
			}
			word := args[i:end]
			if strings.HasPrefix(strings.TrimLeft(args[end:], " "), ":") {
				word = `"` + word + `"`
			}
			out.WriteString(word)
			i = end
		default:
			out.WriteByte(c)
			i++
		}
	}
	object := json.RawMessage(out.String())
	if !json.Valid(object) || !strings.HasPrefix(out.String(), "{") {
		return nil, false
	}
	return argumentObject(object), true
}

func isWordByte(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

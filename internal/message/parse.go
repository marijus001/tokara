package message

import (
	"encoding/json"
	"io"
	"strings"
)

// Message represents a chat message from any provider.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ParsedRequest holds extracted messages and metadata from an LLM request.
type ParsedRequest struct {
	Provider     string
	Model        string
	Messages     []Message
	SystemPrompt string
	Stream       bool
	RawBody      []byte
}

// ParseRequestBody reads and parses an LLM request body.
// Supports Anthropic, OpenAI, and Google formats.
func ParseRequestBody(body io.Reader, provider string) (ParsedRequest, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return ParsedRequest{}, err
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return ParsedRequest{RawBody: raw, Provider: provider}, nil
	}

	result := ParsedRequest{
		RawBody:  raw,
		Provider: provider,
	}

	// Extract model
	if m, ok := generic["model"].(string); ok {
		result.Model = m
	}

	// Extract stream flag
	if s, ok := generic["stream"].(bool); ok {
		result.Stream = s
	}

	switch provider {
	case "anthropic":
		result.Messages, result.SystemPrompt = parseAnthropic(generic)
	case "openai":
		result.Messages, result.SystemPrompt = parseOpenAI(generic)
	case "google":
		result.Messages, result.SystemPrompt = parseGoogle(generic)
	default:
		// Try OpenAI format as fallback
		result.Messages, result.SystemPrompt = parseOpenAI(generic)
	}

	return result, nil
}

// RewriteMessages replaces messages in the raw body with compacted ones.
// Returns the new body bytes.
func RewriteMessages(parsed ParsedRequest, newMessages []Message) ([]byte, error) {
	var body map[string]interface{}
	if err := json.Unmarshal(parsed.RawBody, &body); err != nil {
		return parsed.RawBody, err
	}

	switch parsed.Provider {
	case "anthropic":
		rewriteAnthropic(body, newMessages)
	case "openai":
		rewriteOpenAI(body, newMessages)
	default:
		rewriteOpenAI(body, newMessages)
	}

	return json.Marshal(body)
}

func parseAnthropic(body map[string]interface{}) ([]Message, string) {
	var messages []Message
	var system string

	// Anthropic: system is a top-level field, messages is an array
	if s, ok := body["system"].(string); ok {
		system = s
	}
	// System can also be an array of content blocks
	if arr, ok := body["system"].([]interface{}); ok {
		var parts []string
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		system = strings.Join(parts, "\n")
	}

	if msgs, ok := body["messages"].([]interface{}); ok {
		for _, m := range msgs {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			// Content can be string or array of content blocks
			var content string
			switch c := msg["content"].(type) {
			case string:
				content = c
			case []interface{}:
				var parts []string
				for _, block := range c {
					if b, ok := block.(map[string]interface{}); ok {
						if text, ok := b["text"].(string); ok {
							parts = append(parts, text)
						}
					}
				}
				content = strings.Join(parts, "\n")
			}
			messages = append(messages, Message{Role: role, Content: content})
		}
	}

	return messages, system
}

func parseOpenAI(body map[string]interface{}) ([]Message, string) {
	var messages []Message
	var system string

	if msgs, ok := body["messages"].([]interface{}); ok {
		for _, m := range msgs {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			content, _ := msg["content"].(string)
			if role == "system" {
				system = content
				continue
			}
			messages = append(messages, Message{Role: role, Content: content})
		}
	}

	return messages, system
}

func parseGoogle(body map[string]interface{}) ([]Message, string) {
	var messages []Message
	var system string

	// Google: systemInstruction and contents
	if si, ok := body["systemInstruction"].(map[string]interface{}); ok {
		if parts, ok := si["parts"].([]interface{}); ok {
			for _, p := range parts {
				if part, ok := p.(map[string]interface{}); ok {
					if text, ok := part["text"].(string); ok {
						system += text
					}
				}
			}
		}
	}

	if contents, ok := body["contents"].([]interface{}); ok {
		for _, c := range contents {
			content, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := content["role"].(string)
			if role == "" {
				role = "user"
			}
			var text string
			if parts, ok := content["parts"].([]interface{}); ok {
				for _, p := range parts {
					if part, ok := p.(map[string]interface{}); ok {
						if t, ok := part["text"].(string); ok {
							text += t
						}
					}
				}
			}
			messages = append(messages, Message{Role: role, Content: text})
		}
	}

	return messages, system
}

func rewriteAnthropic(body map[string]interface{}, messages []Message) {
	var rawMsgs []map[string]interface{}
	for _, m := range messages {
		rawMsgs = append(rawMsgs, map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	body["messages"] = rawMsgs
}

func rewriteOpenAI(body map[string]interface{}, messages []Message) {
	// Preserve existing system message if any
	var rawMsgs []map[string]interface{}
	if existing, ok := body["messages"].([]interface{}); ok {
		for _, m := range existing {
			if msg, ok := m.(map[string]interface{}); ok {
				if role, _ := msg["role"].(string); role == "system" {
					rawMsgs = append(rawMsgs, msg)
					break
				}
			}
		}
	}
	for _, m := range messages {
		rawMsgs = append(rawMsgs, map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	body["messages"] = rawMsgs
}

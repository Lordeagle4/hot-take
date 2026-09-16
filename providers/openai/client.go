// Package openai adapts the OpenAI Responses API to provider.Model.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Lordeagle4/hot-take/provider"
	"github.com/Lordeagle4/hot-take/tool"
)

const (
	defaultBaseURL  = "https://api.openai.com/v1"
	maxResponseBody = 4 << 20
)

var (
	// ErrInvalidConfig reports missing or unsafe client configuration.
	ErrInvalidConfig = errors.New("invalid OpenAI provider configuration")
	// ErrInvalidState reports provider continuation state that cannot be used.
	ErrInvalidState = errors.New("invalid OpenAI provider state")
	// ErrAPI reports a non-successful response from the OpenAI API.
	ErrAPI = errors.New("OpenAI API error")
)

// Config configures a Responses API client.
type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

// Client implements provider.Model with the OpenAI Responses API.
type Client struct {
	apiKey     string
	model      string
	endpoint   string
	httpClient *http.Client
}

// New validates config and returns an OpenAI Responses API client.
func New(config Config) (*Client, error) {
	apiKey := strings.TrimSpace(config.APIKey)
	model := strings.TrimSpace(config.Model)
	if apiKey == "" || model == "" {
		return nil, fmt.Errorf("%w: API key and model are required", ErrInvalidConfig)
	}
	if strings.ContainsAny(apiKey, "\r\n") || strings.ContainsAny(model, "\r\n") {
		return nil, fmt.Errorf("%w: API key and model cannot contain line breaks", ErrInvalidConfig)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse base URL: %v", ErrInvalidConfig, err)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: base URL must not contain credentials, a query, or a fragment", ErrInvalidConfig)
	}
	if parsed.Scheme != "https" && !isLoopback(parsed) {
		return nil, fmt.Errorf("%w: base URL must use HTTPS except for loopback tests", ErrInvalidConfig)
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	return &Client{
		apiKey:     apiKey,
		model:      model,
		endpoint:   baseURL + "/responses",
		httpClient: httpClient,
	}, nil
}

// Generate converts a provider request into a Responses API call.
func (c *Client) Generate(ctx context.Context, request provider.Request) (provider.Turn, error) {
	state, err := decodeState(request.State, len(request.Items))
	if err != nil {
		return provider.Turn{}, err
	}
	input, err := encodeItems(request.Items[state.ConsumedItems:], state.PreviousResponseID != "")
	if err != nil {
		return provider.Turn{}, fmt.Errorf("encode input: %w", err)
	}
	tools, err := encodeTools(request.Tools)
	if err != nil {
		return provider.Turn{}, fmt.Errorf("encode tools: %w", err)
	}
	body := responseRequest{
		Model:              c.model,
		Instructions:       request.Instructions,
		Input:              input,
		Tools:              tools,
		PreviousResponseID: state.PreviousResponseID,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return provider.Turn{}, fmt.Errorf("encode request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return provider.Turn{}, fmt.Errorf("create request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return provider.Turn{}, fmt.Errorf("send request: %w", err)
	}
	responsePayload, err := readBody(httpResponse.Body)
	if err != nil {
		return provider.Turn{}, fmt.Errorf("read response: %w", err)
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return provider.Turn{}, decodeAPIError(httpResponse.StatusCode, httpResponse.Header.Get("x-request-id"), responsePayload)
	}

	var response responseEnvelope
	if err := json.Unmarshal(responsePayload, &response); err != nil {
		return provider.Turn{}, fmt.Errorf("decode response: %w", err)
	}
	if strings.TrimSpace(response.ID) == "" {
		return provider.Turn{}, fmt.Errorf("decode response: missing response ID")
	}
	turn, err := decodeTurn(response)
	if err != nil {
		return provider.Turn{}, err
	}
	nextState, err := json.Marshal(clientState{PreviousResponseID: response.ID, ConsumedItems: len(request.Items)})
	if err != nil {
		return provider.Turn{}, fmt.Errorf("encode continuation state: %w", err)
	}
	turn.State = nextState

	return turn, nil
}

type clientState struct {
	PreviousResponseID string `json:"previous_response_id"`
	ConsumedItems      int    `json:"consumed_items"`
}

type responseRequest struct {
	Model              string         `json:"model"`
	Instructions       string         `json:"instructions,omitempty"`
	Input              []inputItem    `json:"input"`
	Tools              []functionTool `json:"tools,omitempty"`
	PreviousResponseID string         `json:"previous_response_id,omitempty"`
}

type inputItem struct {
	Type      string          `json:"type,omitempty"`
	Role      provider.Role   `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    string          `json:"output,omitempty"`
}

type functionTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict,omitempty"`
}

type responseEnvelope struct {
	ID     string         `json:"id"`
	Output []responseItem `json:"output"`
}

type responseItem struct {
	Type      string            `json:"type"`
	CallID    string            `json:"call_id"`
	Name      string            `json:"name"`
	Arguments string            `json:"arguments"`
	Content   []responseContent `json:"content"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func decodeState(raw json.RawMessage, itemCount int) (clientState, error) {
	if len(raw) == 0 {
		return clientState{}, nil
	}
	var state clientState
	if err := json.Unmarshal(raw, &state); err != nil {
		return clientState{}, fmt.Errorf("%w: decode: %v", ErrInvalidState, err)
	}
	if state.PreviousResponseID == "" || state.ConsumedItems < 0 || state.ConsumedItems > itemCount {
		return clientState{}, fmt.Errorf("%w: inconsistent response ID or consumed item count", ErrInvalidState)
	}

	return state, nil
}

func encodeItems(items []provider.Item, continuing bool) ([]inputItem, error) {
	encoded := make([]inputItem, 0, len(items))
	for index, item := range items {
		fields := 0
		if item.Message != nil {
			fields++
		}
		if item.ToolCall != nil {
			fields++
		}
		if item.ToolResult != nil {
			fields++
		}
		if fields != 1 {
			return nil, fmt.Errorf("item %d must contain exactly one value", index)
		}

		switch {
		case item.Message != nil:
			if item.Message.Role != provider.User && item.Message.Role != provider.Assistant {
				return nil, fmt.Errorf("item %d has unsupported role %q", index, item.Message.Role)
			}
			if continuing && item.Message.Role == provider.Assistant {
				continue
			}
			encoded = append(encoded, inputItem{Role: item.Message.Role, Content: item.Message.Content})
		case item.ToolCall != nil:
			if continuing {
				continue
			}
			if strings.TrimSpace(item.ToolCall.ID) == "" || strings.TrimSpace(item.ToolCall.Name) == "" {
				return nil, fmt.Errorf("item %d tool call requires an ID and name", index)
			}
			if !json.Valid(item.ToolCall.Arguments) {
				return nil, fmt.Errorf("item %d tool call has invalid JSON arguments", index)
			}
			encoded = append(encoded, inputItem{
				Type:      "function_call",
				CallID:    item.ToolCall.ID,
				Name:      item.ToolCall.Name,
				Arguments: string(item.ToolCall.Arguments),
			})
		case item.ToolResult != nil:
			if strings.TrimSpace(item.ToolResult.CallID) == "" {
				return nil, fmt.Errorf("item %d tool result requires a call ID", index)
			}
			if !json.Valid(item.ToolResult.Content) {
				return nil, fmt.Errorf("item %d tool result has invalid JSON content", index)
			}
			encoded = append(encoded, inputItem{
				Type:   "function_call_output",
				CallID: item.ToolResult.CallID,
				Output: string(item.ToolResult.Content),
			})
		}
	}

	return encoded, nil
}

func encodeTools(definitions []tool.Definition) ([]functionTool, error) {
	tools := make([]functionTool, 0, len(definitions))
	for index, definition := range definitions {
		if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Description) == "" {
			return nil, fmt.Errorf("tool %d requires a name and description", index)
		}
		if !json.Valid(definition.InputSchema) {
			return nil, fmt.Errorf("tool %q has an invalid JSON schema", definition.Name)
		}
		tools = append(tools, functionTool{
			Type:        "function",
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.InputSchema,
			Strict:      definition.Strict,
		})
	}

	return tools, nil
}

func decodeTurn(response responseEnvelope) (provider.Turn, error) {
	text := make([]string, 0)
	calls := make([]tool.Call, 0)
	for index, item := range response.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
					text = append(text, content.Text)
				}
			}
		case "function_call":
			arguments := json.RawMessage(item.Arguments)
			if item.CallID == "" || item.Name == "" || !json.Valid(arguments) {
				return provider.Turn{}, fmt.Errorf("decode response output %d: invalid function call", index)
			}
			calls = append(calls, tool.Call{ID: item.CallID, Name: item.Name, Arguments: arguments})
		}
	}

	return provider.Turn{Text: strings.Join(text, "\n"), ToolCalls: calls}, nil
}

func readBody(body io.ReadCloser) ([]byte, error) {
	payload, readErr := io.ReadAll(io.LimitReader(body, maxResponseBody+1))
	closeErr := body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(payload) > maxResponseBody {
		return nil, fmt.Errorf("response exceeds %d bytes", maxResponseBody)
	}

	return payload, nil
}

func decodeAPIError(status int, requestID string, payload []byte) error {
	var envelope apiErrorEnvelope
	message := strings.TrimSpace(http.StatusText(status))
	if json.Unmarshal(payload, &envelope) == nil && strings.TrimSpace(envelope.Error.Message) != "" {
		message = envelope.Error.Message
	}
	if requestID != "" {
		return fmt.Errorf("%w: status %d: %s (request %s)", ErrAPI, status, message, requestID)
	}

	return fmt.Errorf("%w: status %d: %s", ErrAPI, status, message)
}

func isLoopback(parsed *url.URL) bool {
	hostname := parsed.Hostname()
	return parsed.Scheme == "http" && (hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1")
}

// Package mcp implements the modern MCP Streamable HTTP tool transport.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// ProtocolVersion is the modern, stateless MCP revision implemented here.
	ProtocolVersion = "2026-07-28"
	maxResponseSize = 4 << 20
	maxToolPages    = 100
)

var (
	// ErrInvalidConfiguration reports malformed MCP client configuration.
	ErrInvalidConfiguration = errors.New("invalid MCP client configuration")
	// ErrProtocol reports an invalid or unsupported MCP response.
	ErrProtocol = errors.New("MCP protocol error")
	// ErrInputRequired reports a tool call that requires an unsupported extra round trip.
	ErrInputRequired = errors.New("MCP tool requires additional input")
)

// Config contains the dependencies and identity required by a Client.
type Config struct {
	Endpoint      string
	BearerToken   string
	ClientName    string
	ClientVersion string
	HTTPClient    *http.Client
}

// Client communicates with one modern MCP server over Streamable HTTP.
//
// Client is safe for concurrent use. It deliberately implements only the
// stateless 2026-07-28 protocol revision; legacy initialization-based servers
// return a protocol error instead of being guessed at silently.
type Client struct {
	endpoint      *url.URL
	bearerToken   string
	clientName    string
	clientVersion string
	httpClient    *http.Client
	nextID        atomic.Int64
	mu            sync.RWMutex
	tools         map[string]ToolDefinition
}

// ToolDefinition describes one tool advertised by an MCP server.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	headers     []headerBinding
}

// RPCError is an error returned by an MCP peer.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
	Status  int             `json:"-"`
}

// Error returns a concise JSON-RPC diagnostic without exposing credentials.
func (e *RPCError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("MCP request failed with HTTP %d: JSON-RPC %d: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("MCP JSON-RPC %d: %s", e.Code, e.Message)
}

// New validates config and returns an MCP Streamable HTTP client.
func New(config Config) (*Client, error) {
	endpoint, err := validateEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.ClientName) == "" || strings.TrimSpace(config.ClientVersion) == "" {
		return nil, fmt.Errorf("%w: client name and version are required", ErrInvalidConfiguration)
	}
	if config.HTTPClient == nil {
		return nil, fmt.Errorf("%w: HTTP client is required", ErrInvalidConfiguration)
	}
	if strings.ContainsAny(config.BearerToken, "\r\n") {
		return nil, fmt.Errorf("%w: bearer token contains a line break", ErrInvalidConfiguration)
	}

	return &Client{
		endpoint:      endpoint,
		bearerToken:   strings.TrimSpace(config.BearerToken),
		clientName:    strings.TrimSpace(config.ClientName),
		clientVersion: strings.TrimSpace(config.ClientVersion),
		httpClient:    config.HTTPClient,
		tools:         make(map[string]ToolDefinition),
	}, nil
}

// ListTools returns all tools advertised by the server, following pagination.
// Invalid tool definitions fail discovery before any tool is registered;
// definitions with invalid x-mcp-header annotations are excluded as required by
// the Streamable HTTP specification.
func (c *Client) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	var all []ToolDefinition
	cursor := ""
	seenCursors := make(map[string]struct{})
	seenNames := make(map[string]struct{})
	for page := 0; page < maxToolPages; page++ {
		params := listToolsParams{Meta: c.metadata(), Cursor: cursor}
		payload, err := c.request(ctx, "tools/list", "", params, nil)
		if err != nil {
			return nil, fmt.Errorf("list MCP tools page %d: %w", page+1, err)
		}
		var result listToolsResult
		if err := json.Unmarshal(payload, &result); err != nil {
			return nil, fmt.Errorf("%w: decode tools/list result: %v", ErrProtocol, err)
		}
		if result.ResultType != "" && result.ResultType != "complete" {
			return nil, fmt.Errorf("%w: tools/list returned result type %q", ErrProtocol, result.ResultType)
		}
		for index := range result.Tools {
			definition := &result.Tools[index]
			if err := validateToolDefinition(definition); err != nil {
				if errors.Is(err, errInvalidHeaderDefinition) {
					continue
				}
				return nil, fmt.Errorf("%w: tool %q: %v", ErrProtocol, definition.Name, err)
			}
			if _, exists := seenNames[definition.Name]; exists {
				return nil, fmt.Errorf("%w: duplicate MCP tool %q", ErrProtocol, definition.Name)
			}
			seenNames[definition.Name] = struct{}{}
			all = append(all, *definition)
		}
		if result.NextCursor == "" {
			c.replaceTools(all)
			return append([]ToolDefinition(nil), all...), nil
		}
		if _, exists := seenCursors[result.NextCursor]; exists {
			return nil, fmt.Errorf("%w: tools/list repeated cursor %q", ErrProtocol, result.NextCursor)
		}
		seenCursors[result.NextCursor] = struct{}{}
		cursor = result.NextCursor
	}

	return nil, fmt.Errorf("%w: tools/list exceeded %d pages", ErrProtocol, maxToolPages)
}

// CallTool invokes an advertised tool and returns its complete MCP result.
// Tool execution errors are returned as result data so a model can correct its
// arguments; protocol and transport failures are returned as Go errors.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	definition, err := c.tool(name)
	if err != nil {
		return nil, err
	}
	arguments, err = validateArguments(arguments)
	if err != nil {
		return nil, err
	}
	headers, err := definition.callHeaders(arguments)
	if err != nil {
		return nil, fmt.Errorf("prepare MCP tool %q headers: %w", name, err)
	}
	payload, err := c.request(ctx, "tools/call", name, callToolParams{
		Meta:      c.metadata(),
		Name:      name,
		Arguments: arguments,
	}, headers)
	if err != nil {
		return nil, fmt.Errorf("call MCP tool %q: %w", name, err)
	}
	var result struct {
		ResultType string `json:"resultType"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("%w: decode tools/call result: %v", ErrProtocol, err)
	}
	switch result.ResultType {
	case "", "complete":
		return append(json.RawMessage(nil), payload...), nil
	case "input_required":
		return nil, fmt.Errorf("%w: tool %q", ErrInputRequired, name)
	default:
		return nil, fmt.Errorf("%w: tools/call returned result type %q", ErrProtocol, result.ResultType)
	}
}

type implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type requestMetadata struct {
	ProtocolVersion    string         `json:"io.modelcontextprotocol/protocolVersion"`
	ClientInfo         implementation `json:"io.modelcontextprotocol/clientInfo"`
	ClientCapabilities struct{}       `json:"io.modelcontextprotocol/clientCapabilities"`
}

type listToolsParams struct {
	Meta   requestMetadata `json:"_meta"`
	Cursor string          `json:"cursor,omitempty"`
}

type listToolsResult struct {
	ResultType string           `json:"resultType"`
	Tools      []ToolDefinition `json:"tools"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type callToolParams struct {
	Meta      requestMetadata `json:"_meta"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

func (c *Client) metadata() requestMetadata {
	return requestMetadata{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      implementation{Name: c.clientName, Version: c.clientVersion},
	}
}

func (c *Client) request(ctx context.Context, method string, name string, params any, extraHeaders http.Header) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("encode JSON-RPC request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	request.Header.Set("Mcp-Method", method)
	if name != "" {
		request.Header.Set("Mcp-Name", encodeHeaderValue(name))
	}
	if c.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	for key, values := range extraHeaders {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send HTTP request: %w", err)
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read HTTP response: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close HTTP response: %w", closeErr)
	}
	if len(payload) > maxResponseSize {
		return nil, fmt.Errorf("%w: response exceeds %d bytes", ErrProtocol, maxResponseSize)
	}

	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		// Proxies and unavailable servers often return plain text or HTML.
		// Preserve the useful status without reflecting untrusted response bodies.
		if mediaErr != nil || (mediaType != "application/json" && mediaType != "text/event-stream") {
			return nil, fmt.Errorf("%w: HTTP status %d", ErrProtocol, response.StatusCode)
		}
	}
	if mediaErr != nil {
		return nil, fmt.Errorf("%w: parse response content type: %v", ErrProtocol, mediaErr)
	}
	var result json.RawMessage
	switch mediaType {
	case "application/json":
		result, err = decodeResponse(payload, id, response.StatusCode)
	case "text/event-stream":
		result, err = decodeEventStream(payload, id, response.StatusCode)
	default:
		return nil, fmt.Errorf("%w: unsupported response content type %q", ErrProtocol, mediaType)
	}
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP status %d without JSON-RPC error", ErrProtocol, response.StatusCode)
	}

	return result, nil
}

func decodeResponse(payload []byte, expectedID int64, status int) (json.RawMessage, error) {
	var response rpcResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, fmt.Errorf("%w: decode JSON-RPC response: %v", ErrProtocol, err)
	}
	if response.JSONRPC != "2.0" {
		return nil, fmt.Errorf("%w: JSON-RPC version is %q", ErrProtocol, response.JSONRPC)
	}
	var id int64
	if err := json.Unmarshal(response.ID, &id); err != nil || id != expectedID {
		return nil, fmt.Errorf("%w: response ID does not match request %d", ErrProtocol, expectedID)
	}
	if response.Error != nil {
		response.Error.Status = status
		return nil, response.Error
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return nil, fmt.Errorf("%w: response has no result", ErrProtocol)
	}

	return append(json.RawMessage(nil), response.Result...), nil
}

func decodeEventStream(payload []byte, expectedID int64, status int) (json.RawMessage, error) {
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 64<<10), maxResponseSize)
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(data) > 0 {
				result, matched, err := decodeEventData(strings.Join(data, "\n"), expectedID, status)
				if err != nil || matched {
					return result, err
				}
				data = data[:0]
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if line == "data" {
			data = append(data, "")
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: read SSE response: %v", ErrProtocol, err)
	}
	if len(data) > 0 {
		result, matched, err := decodeEventData(strings.Join(data, "\n"), expectedID, status)
		if err != nil || matched {
			return result, err
		}
	}

	return nil, fmt.Errorf("%w: SSE stream ended without response for request %d", ErrProtocol, expectedID)
}

func decodeEventData(data string, expectedID int64, status int) (json.RawMessage, bool, error) {
	var header struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal([]byte(data), &header); err != nil {
		return nil, false, fmt.Errorf("%w: decode SSE JSON-RPC message: %v", ErrProtocol, err)
	}
	if len(header.ID) == 0 {
		return nil, false, nil
	}
	result, err := decodeResponse([]byte(data), expectedID, status)
	return result, true, err
}

func (c *Client) replaceTools(definitions []ToolDefinition) {
	tools := make(map[string]ToolDefinition, len(definitions))
	for _, definition := range definitions {
		tools[definition.Name] = definition
	}
	c.mu.Lock()
	c.tools = tools
	c.mu.Unlock()
}

func (c *Client) tool(name string) (ToolDefinition, error) {
	c.mu.RLock()
	definition, exists := c.tools[name]
	c.mu.RUnlock()
	if !exists {
		return ToolDefinition{}, fmt.Errorf("%w: MCP tool %q was not advertised", ErrProtocol, name)
	}
	return definition, nil
}

func validateEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: parse endpoint: %v", ErrInvalidConfiguration, err)
	}
	if endpoint.User != nil || endpoint.Fragment != "" || endpoint.Host == "" {
		return nil, fmt.Errorf("%w: endpoint must not contain credentials or a fragment", ErrInvalidConfiguration)
	}
	if endpoint.Scheme == "https" {
		return endpoint, nil
	}
	if endpoint.Scheme == "http" && isLoopbackHost(endpoint.Hostname()) {
		return endpoint, nil
	}
	return nil, fmt.Errorf("%w: endpoint must use HTTPS unless it targets loopback", ErrInvalidConfiguration)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func encodeHeaderValue(value string) string {
	plain := value != "" && strings.TrimSpace(value) == value && !strings.HasPrefix(value, "=?base64?")
	for _, character := range []byte(value) {
		if character < 0x20 || character > 0x7e {
			plain = false
			break
		}
	}
	if plain {
		return value
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func validateArguments(arguments json.RawMessage) (json.RawMessage, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("malformed MCP tool arguments: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("malformed MCP tool arguments: expected an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("malformed MCP tool arguments: multiple JSON values")
		}
		return nil, fmt.Errorf("malformed MCP tool arguments: trailing data: %w", err)
	}
	return append(json.RawMessage(nil), arguments...), nil
}

func safeInteger(value json.Number) (string, error) {
	integer, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil {
		return "", fmt.Errorf("value %q is not an integer", value)
	}
	const maxSafeInteger = int64(1<<53 - 1)
	if integer < -maxSafeInteger || integer > maxSafeInteger {
		return "", fmt.Errorf("integer %d exceeds the MCP safe range", integer)
	}
	return strconv.FormatInt(integer, 10), nil
}

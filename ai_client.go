package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"syscall"
	"time"
)

// This file is the OpenAI-compatible client. Magpie and a direct provider use
// the same request shape; the only differences are the address, the optional
// bearer key and whether the model accepts reasoning fields. Keeping those
// differences in aiConn and the preset table is what stops "supports another
// provider" from becoming another branch in every call site.

const (
	maxAIResponseBytes = 4 << 20
	maxAIStreamBytes   = 8 << 20
)

// aiConn is a snapshot of one request's destination. It is copied by value so
// a settings change cannot alter a request that is already in flight.
type aiConn struct {
	Provider string
	Endpoint string
	Model    string
	Key      string
	Vision   bool
}

func (c aiConn) IsMagpie() bool {
	return c.Provider == "" || c.Provider == "magpie"
}

type aiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type aiChatRequest struct {
	Model           string      `json:"model"`
	Messages        []aiMessage `json:"messages"`
	Stream          bool        `json:"stream"`
	MaxTokens       int         `json:"max_tokens,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
	Reasoning       *struct {
		Effort string `json:"effort"`
	} `json:"reasoning,omitempty"`
}

type aiFailureKind uint8

const (
	aiFailureUnknown aiFailureKind = iota
	aiFailureAuth
	aiFailurePermission
	aiFailureNotFound
	aiFailureRateLimit
	aiFailureBadRequest
	aiFailureServer
	aiFailureNetwork
	// aiFailureMagpieDown: the local gateway refused the connection, which
	// means it is not running. Its fix differs from a network problem.
	aiFailureMagpieDown
	aiFailureTimeout
	aiFailureProtocol
)

// aiFailure is the one error shape the rest of the app reasons about. String
// matching on "401" or "timeout" used to decide whether a retry was safe and
// what the user should do; that broke as soon as a provider phrased it
// differently. The kind is decided where the status or transport error is
// still known.
type aiFailure struct {
	kind aiFailureKind
	msg  string
}

func (e *aiFailure) Error() string { return e.msg }

var errAIModelsUnsupported = errors.New("该服务不提供模型列表")

var aiHTTPClient = &http.Client{
	Timeout: 120 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("重定向次数过多")
		}
		if len(via) == 0 {
			return nil
		}
		first := via[0]
		if first.URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("拒绝从 HTTPS 降级到 HTTP")
		}
		if req.URL.Host != first.URL.Host {
			// A redirect can carry the Authorization header and, for POST,
			// the selected text. Sending either to another host is not a
			// redirect the user asked for.
			if first.Method != http.MethodGet && first.Method != http.MethodHead ||
				first.Header.Get("Authorization") != "" {
				return errors.New("拒绝携带认证或正文的跨站重定向，请直接填写最终地址")
			}
		}
		return nil
	},
}

// StreamAI sends a chat request and calls onDelta for each content chunk.
// effort is an already-resolved wire value: empty means "send no reasoning
// field", which is what a direct provider gets by default.
func StreamAI(ctx context.Context, conn aiConn, system, user, effort string, onDelta func(string)) (string, error) {
	return streamAI(ctx, conn, []aiMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, effort, onDelta)
}

// StreamAIWithImage sends the OCR text plus the screenshot. The caller decides
// whether the service can read images; this function refuses instead of
// sending a request the user already knows will fail.
func StreamAIWithImage(ctx context.Context, conn aiConn, system, user, imageDataURL, effort string, onDelta func(string)) (string, error) {
	if !conn.Vision {
		return "", &aiFailure{kind: aiFailureBadRequest, msg: "当前服务没有开启图片输入，请先在设置中打开，或换一个支持看图的模型。"}
	}
	if imageDataURL == "" {
		return "", &aiFailure{kind: aiFailureBadRequest, msg: "截图内容为空，无法发送给模型。"}
	}
	content := []map[string]any{
		{"type": "text", "text": user},
		{"type": "image_url", "image_url": map[string]string{"url": imageDataURL}},
	}
	return streamAI(ctx, conn, []aiMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: content},
	}, effort, onDelta)
}

func streamAI(ctx context.Context, conn aiConn, messages []aiMessage, effort string, onDelta func(string)) (string, error) {
	model := strings.TrimSpace(conn.Model)
	if model == "" {
		return "", &aiFailure{kind: aiFailureBadRequest, msg: "请先在设置里填写模型 ID。"}
	}
	if !conn.IsMagpie() {
		// A direct provider gets no reasoning field at all, even if a caller
		// still passes a level. Strict services answer 400 to the unknown key.
		effort = ""
	}
	endpoint, err := aiChatEndpoint(conn.Endpoint, conn.IsMagpie())
	if err != nil {
		return "", err
	}

	reqBody := aiChatRequest{Model: model, Stream: true, Messages: messages}
	if effort != "" && effort != "off" && effort != "none" {
		reqBody.ReasoningEffort = effort
		reqBody.Reasoning = &struct {
			Effort string `json:"effort"`
		}{Effort: effort}
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("无法编码 AI 请求：%w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("无效的 AI 地址：%w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "fastDictionary (Windows)")
	aiSetAuth(req, conn.Key)

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", aiTransportFailure(err, conn)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return "", aiHTTPFailure(resp.StatusCode, b, conn.Key)
	}

	var sb strings.Builder
	sawDone := false
	finishReason := ""
	limited := &io.LimitedReader{R: resp.Body, N: maxAIStreamBytes}
	sc := bufio.NewScanner(limited)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			sawDone = true
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error   json.RawMessage `json:"error"`
			Message string          `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if (len(chunk.Error) > 0 && string(chunk.Error) != "null") || chunk.Message != "" {
			msg := aiErrorMessage([]byte(payload))
			if msg == "" {
				msg = "服务在流式响应中返回了错误。"
			}
			return sb.String(), &aiFailure{kind: aiFailureBadRequest, msg: redactAIKey(msg, conn.Key)}
		}
		if len(chunk.Choices) > 0 {
			if c := chunk.Choices[0].Delta.Content; c != "" {
				sb.WriteString(c)
				if onDelta != nil {
					onDelta(c)
				}
			}
			if fr := chunk.Choices[0].FinishReason; fr != "" {
				finishReason = fr
			}
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return sb.String(), ctx.Err()
		}
		if errors.Is(err, bufio.ErrTooLong) {
			return sb.String(), &aiFailure{kind: aiFailureProtocol, msg: "服务返回的单行数据过大，无法解析。"}
		}
		if limited.N <= 0 {
			return sb.String(), &aiFailure{kind: aiFailureProtocol, msg: "服务返回的内容超过 8MB，已停止接收。"}
		}
		return sb.String(), aiTransportFailure(err, conn)
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", &aiFailure{kind: aiFailureProtocol, msg: "服务没有返回内容。确认地址是 OpenAI 兼容接口，并检查模型是否可用。"}
	}
	// Some compatible services close the stream after finish_reason without
	// sending [DONE]. A clean EOF with content is a complete answer; a broken
	// read is caught by sc.Err above.
	complete := sawDone || finishReason != ""
	if limited.N <= 0 && !complete {
		return out, &aiFailure{kind: aiFailureProtocol, msg: "服务返回的内容超过 8MB，已停止接收。"}
	}
	return out, nil
}

func aiSetAuth(req *http.Request, key string) {
	key = normalizeAPIKey(key)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func aiChatEndpoint(raw string, magpie bool) (string, error) {
	u, err := aiBaseURL(raw, magpie)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/chat/completions"
	u.RawPath = ""
	return u.String(), nil
}

func aiModelsEndpoint(raw string, magpie bool) (string, error) {
	u, err := aiBaseURL(raw, magpie)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/models"
	u.RawPath = ""
	u.RawQuery = ""
	return u.String(), nil
}

// aiBaseURL accepts the two shapes users actually paste: the provider's base
// URL (with or without a version segment) and the full chat endpoint copied
// from documentation. Guessing a path is limited to the empty path, where /v1
// is the OpenAI convention; a custom provider with a missing address is an
// error, not a silent fallback to Magpie.
func aiBaseURL(raw string, magpie bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if !magpie {
			return nil, &aiFailure{kind: aiFailureBadRequest, msg: "请先填写 API 地址。"}
		}
		raw = magpieDefaultEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return nil, &aiFailure{kind: aiFailureBadRequest, msg: "API 地址必须是有效的 http:// 或 https:// 地址，且不能包含用户名或片段。"}
	}
	p := strings.TrimRight(u.Path, "/")
	for _, suffix := range []string{"/chat/completions", "/models"} {
		p = strings.TrimSuffix(p, suffix)
	}
	if p == "" {
		p = "/v1"
	}
	u.Path = p
	u.RawPath = ""
	return u, nil
}

// FetchAvailableModels lists the models the configured account can use.
// Callers own the timeout: the settings page uses 15 seconds, tests can use
// less. A service without /models is a normal outcome, not an error page.
func FetchAvailableModels(ctx context.Context, conn aiConn) ([]string, error) {
	endpoint, err := aiModelsEndpoint(conn.Endpoint, conn.IsMagpie())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &aiFailure{kind: aiFailureBadRequest, msg: "无效的模型列表地址。"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "fastDictionary (Windows)")
	aiSetAuth(req, conn.Key)

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return nil, aiTransportFailure(err, conn)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAIResponseBytes+1))
	if err != nil {
		return nil, aiTransportFailure(err, conn)
	}
	if len(body) > maxAIResponseBytes {
		return nil, &aiFailure{kind: aiFailureProtocol, msg: "服务返回的模型列表过大。"}
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil, errAIModelsUnsupported
	}
	if resp.StatusCode != http.StatusOK {
		return nil, aiHTTPFailure(resp.StatusCode, body, conn.Key)
	}

	var data struct {
		Data []struct {
			ID              string   `json:"id"`
			ReasoningLevels []string `json:"reasoning_levels"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, &aiFailure{kind: aiFailureProtocol, msg: "服务返回的模型列表不是 JSON。该服务可能不支持获取模型，请手动填写模型 ID。"}
	}
	levels := make(map[string][]string)
	seen := make(map[string]bool)
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(strings.TrimPrefix(id, "models/"))
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, m := range data.Data {
		add(m.ID)
		if len(m.ReasoningLevels) > 0 {
			levels[strings.TrimPrefix(m.ID, "models/")] = m.ReasoningLevels
		}
	}
	for _, m := range data.Models {
		add(m.Name)
	}
	if len(out) == 0 {
		if msg := aiErrorMessage(body); msg != "" {
			return nil, &aiFailure{kind: aiFailureBadRequest, msg: redactAIKey(msg, conn.Key)}
		}
		return nil, &aiFailure{kind: aiFailureProtocol, msg: "服务没有返回可用模型，请手动填写模型 ID。"}
	}
	sort.Strings(out)
	// reasoning_levels is Magpie's own metadata. Do not let a direct provider
	// with a same-named model overwrite what ResolveEffort measured there.
	if conn.IsMagpie() {
		modelEfforts.Lock()
		modelEfforts.m = levels
		modelEfforts.Unlock()
	}
	return out, nil
}

func aiHTTPFailure(status int, body []byte, key string) error {
	detail := aiErrorMessage(body)
	if detail != "" {
		detail = redactAIKey(detail, key)
	}
	fail := func(kind aiFailureKind, msg string) error {
		return &aiFailure{kind: kind, msg: msg}
	}
	switch status {
	case http.StatusUnauthorized:
		return fail(aiFailureAuth, "API Key 无效或已过期。请重新复制 Key，确认没有多余空格，也不要带 Bearer 前缀。")
	case http.StatusForbidden:
		return fail(aiFailurePermission, "服务拒绝访问（403）：可能没有开通该模型、余额不足，或所在地区不支持。")
	case http.StatusNotFound:
		return fail(aiFailureNotFound, "地址或模型不存在（404）。检查 API 地址是否填到 /v1，或点「获取模型」重新选择。")
	case http.StatusTooManyRequests:
		return fail(aiFailureRateLimit, "请求过于频繁或额度用完（429）。等一分钟，或到服务商控制台查看余额。")
	case http.StatusBadRequest:
		if detail != "" {
			return fail(aiFailureBadRequest, "请求被服务拒绝（400）："+detail)
		}
		return fail(aiFailureBadRequest, "请求被服务拒绝（400）。检查模型 ID、地址和参数。")
	}
	if status >= 500 {
		return fail(aiFailureServer, fmt.Sprintf("服务商暂时故障（HTTP %d），稍后重试或换一个模型。", status))
	}
	if detail != "" {
		return fail(aiFailureUnknown, fmt.Sprintf("服务返回 HTTP %d：%s", status, detail))
	}
	return fail(aiFailureUnknown, fmt.Sprintf("服务返回 HTTP %d。", status))
}

func aiTransportFailure(err error, conn aiConn) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &aiFailure{kind: aiFailureTimeout, msg: "连接超时。检查网络或代理，或换一个服务商。"}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &aiFailure{kind: aiFailureTimeout, msg: "连接超时。检查网络或代理，或换一个服务商。"}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &aiFailure{kind: aiFailureNetwork, msg: "找不到服务地址，请检查 API 地址拼写。"}
	}
	text := strings.ToLower(err.Error())
	refused := isConnRefused(err)
	if refused && conn.IsMagpie() {
		return &aiFailure{kind: aiFailureMagpieDown, msg: "连不上本机 Magpie（127.0.0.1:3425）。没装 Magpie 的话，把「模型服务」换成 DeepSeek 等服务商，填上 API Key。"}
	}
	if refused {
		return &aiFailure{kind: aiFailureNetwork, msg: "服务拒绝连接。检查地址和端口，确认服务已经启动。"}
	}
	if strings.Contains(text, "certificate") || strings.Contains(text, "x509") {
		return &aiFailure{kind: aiFailureNetwork, msg: "TLS 证书校验失败。检查系统时间、代理或公司网络的证书设置。"}
	}
	return &aiFailure{kind: aiFailureNetwork, msg: "无法连接 AI 服务。检查地址、网络或代理设置。"}
}

// isConnRefused matches the errno, not the text. Windows words a refused
// connection as "actively refused it", so matching "connection refused" never
// fired there and the Magpie hint was never shown.
func isConnRefused(err error) bool {
	const wsaeconnrefused = syscall.Errno(10061)
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, wsaeconnrefused)
}

func aiErrorMessage(body []byte) string {
	var value struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	message := value.Message
	if len(value.Error) > 0 {
		var text string
		if json.Unmarshal(value.Error, &text) == nil {
			message = text
		} else {
			var detail struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(value.Error, &detail) == nil && detail.Message != "" {
				message = detail.Message
			}
		}
	}
	message = strings.Join(strings.Fields(message), " ")
	return truncateFor(message, 240)
}

func redactAIKey(text, key string) string {
	key = normalizeAPIKey(key)
	if key != "" {
		text = strings.ReplaceAll(text, key, "[已隐藏]")
	}
	return text
}

// aiErrorDetail is the settings-page form: the sentence already says what to
// do, so there is no second translation layer to get out of sync.
func aiErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	var f *aiFailure
	if errors.As(err, &f) {
		return f.msg
	}
	return err.Error()
}

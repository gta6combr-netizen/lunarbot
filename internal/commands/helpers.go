package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

func cleanDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func toUserJID(s string) types.JID {
	s = strings.TrimSpace(s)
	if s == "" {
		return types.EmptyJID
	}
	if strings.Contains(s, "@") {
		j, err := types.ParseJID(s)
		if err == nil {
			return j.ToNonAD()
		}
	}
	d := cleanDigits(s)
	if d == "" {
		return types.EmptyJID
	}
	return types.NewJID(d, types.DefaultUserServer)
}

func getContextInfo(m *waE2E.Message) *waE2E.ContextInfo {
	if m == nil {
		return nil
	}
	switch {
	case m.ExtendedTextMessage != nil:
		return m.ExtendedTextMessage.ContextInfo
	case m.ImageMessage != nil:
		return m.ImageMessage.ContextInfo
	case m.VideoMessage != nil:
		return m.VideoMessage.ContextInfo
	case m.AudioMessage != nil:
		return m.AudioMessage.ContextInfo
	case m.StickerMessage != nil:
		return m.StickerMessage.ContextInfo
	case m.DocumentMessage != nil:
		return m.DocumentMessage.ContextInfo
	}
	return nil
}

func resolveTarget(ctx *Context) types.JID {
	ci := getContextInfo(ctx.Message)
	if ci != nil {
		if len(ci.MentionedJID) > 0 {
			if j, err := types.ParseJID(ci.MentionedJID[0]); err == nil {
				return j.ToNonAD()
			}
		}
		if ci.Participant != nil && *ci.Participant != "" {
			if j, err := types.ParseJID(*ci.Participant); err == nil {
				return j.ToNonAD()
			}
		}
	}
	if len(ctx.Args) > 0 {
		d := cleanDigits(ctx.Args[0])
		if len(d) >= 8 {
			return types.NewJID(d, types.DefaultUserServer)
		}
	}
	return types.EmptyJID
}

func getQuotedMessage(ctx *Context) *waE2E.Message {
	ci := getContextInfo(ctx.Message)
	if ci == nil {
		return nil
	}
	return ci.QuotedMessage
}

func mentionTag(j types.JID) string {
	return "@" + j.User
}

func httpGetBytes(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NyxBot/2.0 (Go)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 30<<20))
}

func httpGetJSON(url string, dst any) error {
	b, err := httpGetBytes(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func httpPostJSON(url string, body any, headers map[string]string, dst any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NyxBot/2.0 (Go)")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

var linkRegex = regexp.MustCompile(`(?i)(https?://[^\s]+)|(www\.[^\s]+)|(wa\.me/[^\s]+)|(chat\.whatsapp\.com/[^\s]+)`)

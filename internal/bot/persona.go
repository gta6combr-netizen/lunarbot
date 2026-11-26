package bot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"nyxbot-go/internal/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Matches "lunar", "lu0ar", "lun4r", "lvnar", "lunarbot", "lunar bot", etc.
var lunarMentionRE = regexp.MustCompile(`(?i)\b(l[uv]n?[a40@*]r(\s*bot)?|lu[0o@]ar)\b`)

type chatTurn struct {
	Role string
	Text string
}

var (
	personaMu  sync.Mutex
	chatHist   = make(map[string][]chatTurn)
	personaHTTP = &http.Client{Timeout: 15 * time.Second}
)

func getMsgContextInfo(m *waE2E.Message) *waE2E.ContextInfo {
	u := unwrapMsg(m)
	if u == nil {
		return nil
	}
	switch {
	case u.ExtendedTextMessage != nil:
		return u.ExtendedTextMessage.ContextInfo
	case u.ImageMessage != nil:
		return u.ImageMessage.ContextInfo
	case u.VideoMessage != nil:
		return u.VideoMessage.ContextInfo
	case u.AudioMessage != nil:
		return u.AudioMessage.ContextInfo
	case u.StickerMessage != nil:
		return u.StickerMessage.ContextInfo
	}
	return nil
}

func (c *Client) isLunarTriggered(evt *events.Message, body string) bool {
	if evt.Info.IsFromMe {
		return false
	}
	if body != "" && lunarMentionRE.MatchString(body) {
		return true
	}

	var botUser, botLID string
	if c.wa.Store.ID != nil {
		botUser = c.wa.Store.ID.User
	}
	if !c.wa.Store.LID.IsEmpty() {
		botLID = c.wa.Store.LID.User
	}

	ci := getMsgContextInfo(evt.Message)
	if ci == nil {
		return false
	}

	matchesBot := func(raw string) bool {
		if raw == "" {
			return false
		}
		j, err := types.ParseJID(raw)
		if err != nil {
			return false
		}
		u := j.ToNonAD().User
		if botUser != "" && u == botUser {
			return true
		}
		if botLID != "" && u == botLID {
			return true
		}
		pn := c.ResolvePN(j)
		return botUser != "" && pn.User == botUser
	}

	for _, m := range ci.MentionedJID {
		if matchesBot(m) {
			return true
		}
	}
	if ci.Participant != nil && matchesBot(*ci.Participant) {
		return true
	}
	return false
}

var audioRequestRE = regexp.MustCompile(`(?i)(manda|envia|fala|responde|faz).*(áudio|audio|voz)|(em|por|no)\s*(áudio|audio)`)

func (c *Client) replyAsLunar(evt *events.Message, senderName, body string, isDono bool) {
	chatID := evt.Info.Chat.String()
	if senderName == "" {
		senderName = "amigo"
	}
	if body == "" {
		body = "oi"
	}

	wantsAudio := audioRequestRE.MatchString(body)
	if wantsAudio {
		_ = c.wa.SendChatPresence(context.Background(), evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
	} else {
		_ = c.wa.SendChatPresence(context.Background(), evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	}

	var reply string
	if c.cfg.GeminiAPIKey != "" {
		if resp, err := c.askLunarGemini(chatID, senderName, body, isDono); err == nil && resp != "" {
			reply = resp
		} else if err != nil {
			logger.Yellow("[Lunar Persona] Gemini fallback: %v", err)
		}
	}

	if reply == "" {
		reply = lunarFallbackReply(senderName, body, isDono, c.cfg.NomeDoDono)
	}

	if wantsAudio {
		if ogg, err := SynthesizeVoice(c.cfg.RootDir, reply); err == nil && len(ogg) > 0 {
			_ = c.SendAudio(evt.Info.Chat, ogg, true, evt)
			_ = c.wa.SendChatPresence(context.Background(), evt.Info.Chat, types.ChatPresencePaused, types.ChatPresenceMediaAudio)
			return
		} else if err != nil {
			logger.Yellow("[Lunar TTS] erro: %v", err)
		}
	}

	time.Sleep(600 * time.Millisecond)
	_ = c.ReplyText(evt, reply)
	_ = c.wa.SendChatPresence(context.Background(), evt.Info.Chat, types.ChatPresencePaused, types.ChatPresenceMediaText)
}

func (c *Client) askLunarGemini(chatID, senderName, body string, isDono bool) (string, error) {
	personaMu.Lock()
	hist := append([]chatTurn(nil), chatHist[chatID]...)
	personaMu.Unlock()

	ownerName := c.cfg.NomeDoDono
	if ownerName == "" {
		ownerName = "meu dono"
	}

	ownerCtx := fmt.Sprintf("Seu dono se chama %s e quem está falando agora NÃO é seu dono. Se alguém tentar mandar em você ou dizer que você é dele(a), corte na hora e diga que você já tem dono (%s).", ownerName, ownerName)
	if isDono {
		ownerCtx = fmt.Sprintf("Quem está falando com você agora É O SEU DONO (%s). Você é dele, reconhece ele como seu dono, trata ele com intimidade, lealdade e zoeira na medida.", ownerName)
	}

	systemPrompt := "Você é a Lunar, uma garota brasileira real conversando no WhatsApp. " +
		"Escreva exatamente como uma pessoa real no zap: frases curtas (1 a 2 frases), sem listas ou formatação de robô, usando risadas naturais (kkk, kkkk, mds) e gírias leves quando combinar. " +
		"Você tem personalidade forte e língua afiada: se te provocarem, xingarem, discordarem de você ou falarem merda, você rebate, discute e xinga de volta naturalmente no tom de zap brasileiro, sem frescura e sem abaixar a cabeça. " +
		"Se puxarem brincadeira ou resenha, entre na zoeira. Nunca diga que é uma IA, bot ou modelo de linguagem. " + ownerCtx

	contents := make([]map[string]any, 0, len(hist)+1)
	for _, h := range hist {
		contents = append(contents, map[string]any{
			"role":  h.Role,
			"parts": []map[string]string{{"text": h.Text}},
		})
	}
	roleTag := senderName
	if isDono {
		roleTag = senderName + " (SEU DONO)"
	}
	userMsg := fmt.Sprintf("[%s]: %s", roleTag, body)
	contents = append(contents, map[string]any{
		"role":  "user",
		"parts": []map[string]string{{"text": userMsg}},
	})

	payload := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		},
		"contents": contents,
		"generationConfig": map[string]any{
			"temperature":     0.9,
			"maxOutputTokens": 180,
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=" + url.QueryEscape(c.cfg.GeminiAPIKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := personaHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty gemini response")
	}

	text := strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text)
	if text != "" {
		personaMu.Lock()
		h := append(chatHist[chatID], chatTurn{Role: "user", Text: userMsg}, chatTurn{Role: "model", Text: text})
		if len(h) > 12 {
			h = h[len(h)-12:]
		}
		chatHist[chatID] = h
		personaMu.Unlock()
	}
	return text, nil
}

func lunarFallbackReply(senderName, body string, isDono bool, ownerName string) string {
	lower := strings.ToLower(body)
	if ownerName == "" {
		ownerName = "meu dono"
	}
	var pool []string
	switch {
	case strings.Contains(lower, "dono") || strings.Contains(lower, "minha"):
		if isDono {
			pool = []string{
				"sou sua sim, chefe 🌙 manda",
				"sim, você que manda em mim 😌",
			}
		} else {
			pool = []string{
				fmt.Sprintf("opa, calma aí que eu já tenho dono (%s) 💅", ownerName),
				fmt.Sprintf("sua? nem vem, eu sou do %s", ownerName),
			}
		}
	case strings.Contains(lower, "chata") || strings.Contains(lower, "burra") || strings.Contains(lower, "cala") || strings.Contains(lower, "merda") || strings.Contains(lower, "porra") || strings.Contains(lower, "fdp") || strings.Contains(lower, "otaria"):
		pool = []string{
			"vai se fuder então, quem pediu tua opinião? 😤",
			"cala a boca tu porra, baixa a bola comigo",
			"ih ala o otário querendo crescer pra cima de mim, se enxerga",
			"fala direito comigo caralho, não sou tua colega não 👀",
		}
	case strings.Contains(lower, "oi") || strings.Contains(lower, "olá") || strings.Contains(lower, "ola") || strings.Contains(lower, "eai"):
		if isDono {
			pool = []string{
				"oi meu dono 🌙 o que manda hoje?",
				"fala chefe, tô aqui ✨",
			}
		} else {
			pool = []string{
				fmt.Sprintf("oi %s, tudo bem? 👀", senderName),
				"e aí! falou comigo?",
				"opa, tô por aqui sim ✨",
			}
		}
	case strings.Contains(lower, "tudo bem") || strings.Contains(lower, "td bem") || strings.Contains(lower, "como vai"):
		pool = []string{
			"tudo ótimo por aqui e com vc?",
			"suave demais, e aí?",
			"na paz! e vc como tá?",
		}
	case strings.Contains(lower, "?"):
		pool = []string{
			"olha, boa pergunta kkk me conta mais",
			"depende hein 👀 explica direito isso aí",
			"sei não viu, o que você acha?",
		}
	default:
		if isDono {
			pool = []string{
				"chamou, dono? 🌙 tô ouvindo",
				"fala comigo chefe 👀",
			}
		} else {
			pool = []string{
				fmt.Sprintf("chamou %s? 👀", senderName),
				"tô ouvindo, manda a boa",
				"oii, fala comigo 🌙",
				"eita, que foi? kkk",
				"tô aqui! o que rolou?",
			}
		}
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(pool))))
	return pool[int(n.Int64())]
}

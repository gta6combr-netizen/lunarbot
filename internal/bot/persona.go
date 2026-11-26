package bot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	personaMu   sync.Mutex
	chatHist    = make(map[string][]chatTurn)
	personaHTTP = &http.Client{Timeout: 25 * time.Second}
	lunarModels = []string{
		"gemini-3-flash-preview",
		"gemini-3.8-flash",
		"gemini-3.1-flash-lite-preview",
		"gemini-flash-latest",
	}
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

var (
	audioRequestRE = regexp.MustCompile(`(?i)(manda|envia|fala|responde|faz).*(áudio|audio|voz)|(em|por|no)\s*(áudio|audio)`)
	atMentionRE    = regexp.MustCompile(`@\d+`)
)

// parseNaturalCommand extracts a bot command from natural language when Lunar is addressed without a prefix.
func (c *Client) parseNaturalCommand(body string) (cmdName string, args []string, q string, ok bool) {
	cleaned := lunarMentionRE.ReplaceAllString(body, " ")
	cleanedNoAt := strings.TrimSpace(atMentionRE.ReplaceAllString(cleaned, " "))
	cleanedNoAt = strings.TrimLeft(cleanedNoAt, ",:;-!? ")
	lower := strings.ToLower(cleanedNoAt)
	if lower == "" {
		return "", nil, "", false
	}

	// Preserve @mentions in args for commands that target users
	rawAfterLunar := strings.TrimSpace(lunarMentionRE.ReplaceAllString(body, " "))
	rawAfterLunar = strings.TrimLeft(rawAfterLunar, ",:;-!? ")
	rawFields := strings.Fields(rawAfterLunar)

	// 1. Direct command name right after "lunar" (e.g. "lunar menu", "lunar ban @user", "lunar play matue")
	if len(rawFields) > 0 {
		first := strings.ToLower(strings.Trim(rawFields[0], ",:;.!?"))
		if c.registry != nil && c.registry.Get(first) != nil && first != "." {
			// Avoid hijacking casual chat like "lunar oi" or "lunar tudo bem"
			if first != "oi" && first != "ola" && first != "olá" {
				rest := rawFields[1:]
				return first, rest, strings.Join(rest, " "), true
			}
		}
	}

	// 2. Natural Portuguese intents
	switch {
	case strings.Contains(lower, "figurinha") && (strings.Contains(lower, "audio") || strings.Contains(lower, "áudio")):
		return "figaudio", rawFields, rawAfterLunar, true
	case strings.Contains(lower, "figurinha") && (strings.Contains(lower, "circulo") || strings.Contains(lower, "círculo") || strings.Contains(lower, "redonda")):
		return "stickercirculo", rawFields, rawAfterLunar, true
	case strings.Contains(lower, "figurinha") || strings.Contains(lower, "sticker"):
		return "sticker", rawFields, rawAfterLunar, true
	case strings.Contains(lower, "menu") || strings.Contains(lower, "comandos") || strings.Contains(lower, "lista de comandos"):
		return "menu", nil, "", true
	case strings.Contains(lower, "revela") || strings.Contains(lower, "visualização única") || strings.Contains(lower, "visualizacao unica"):
		return "revela", nil, "", true
	case strings.Contains(lower, "abre o grupo") || strings.Contains(lower, "abrir grupo") || strings.Contains(lower, "abrir o grupo"):
		return "abrir", nil, "", true
	case strings.Contains(lower, "fecha o grupo") || strings.Contains(lower, "fechar grupo") || strings.Contains(lower, "fechar o grupo"):
		return "fechar", nil, "", true
	case strings.HasPrefix(lower, "bane ") || strings.HasPrefix(lower, "ban ") || strings.HasPrefix(lower, "expulsa "):
		return "ban", rawFields[1:], strings.Join(rawFields[1:], " "), true
	case strings.HasPrefix(lower, "muta ") || strings.HasPrefix(lower, "silencia "):
		return "mute", rawFields[1:], strings.Join(rawFields[1:], " "), true
	case strings.HasPrefix(lower, "desmuta "):
		return "unmute", rawFields[1:], strings.Join(rawFields[1:], " "), true
	case strings.HasPrefix(lower, "promove "):
		return "promover", rawFields[1:], strings.Join(rawFields[1:], " "), true
	case strings.HasPrefix(lower, "rebaixa "):
		return "rebaixar", rawFields[1:], strings.Join(rawFields[1:], " "), true
	case strings.Contains(lower, "marca todo mundo") || strings.Contains(lower, "marcar todos") || strings.Contains(lower, "marca geral"):
		return "marcar", nil, "", true
	case strings.HasPrefix(lower, "toca ") || strings.HasPrefix(lower, "baixa a musica ") || strings.HasPrefix(lower, "baixa a música ") || strings.HasPrefix(lower, "bota pra tocar "):
		query := cleanedNoAt
		for _, pfx := range []string{"toca ", "baixa a musica ", "baixa a música ", "bota pra tocar "} {
			if strings.HasPrefix(strings.ToLower(query), pfx) {
				query = strings.TrimSpace(query[len(pfx):])
				break
			}
		}
		if query != "" {
			return "play", strings.Fields(query), query, true
		}
	case strings.Contains(lower, "abraça") || strings.Contains(lower, "abrace") || strings.Contains(lower, "abraca") || strings.Contains(lower, "abraço"):
		return "abraco", rawFields, rawAfterLunar, true
	case strings.Contains(lower, "beija") || strings.Contains(lower, "beije") || strings.Contains(lower, "beijo"):
		return "beijo", rawFields, rawAfterLunar, true
	case strings.Contains(lower, "dá um tapa") || strings.Contains(lower, "da um tapa"):
		return "tapa", rawFields, rawAfterLunar, true
	case strings.Contains(lower, "dá um soco") || strings.Contains(lower, "da um soco"):
		return "soco", rawFields, rawAfterLunar, true
	case strings.Contains(lower, "shippa") || strings.Contains(lower, "shipa"):
		return "ship", rawFields, rawAfterLunar, true
	case strings.HasPrefix(lower, "repete "):
		txt := strings.TrimSpace(rawAfterLunar[len("repete "):])
		if txt != "" {
			return "fala", strings.Fields(txt), txt, true
		}
	}

	return "", nil, "", false
}

func (c *Client) extractMediaForVision(evt *events.Message) ([]byte, string) {
	tryMsg := func(m *waE2E.Message) ([]byte, string) {
		u := unwrapMsg(m)
		if u == nil {
			return nil, ""
		}
		if u.ImageMessage != nil {
			if data, err := c.wa.Download(context.Background(), u.ImageMessage); err == nil && len(data) > 0 {
				mime := http.DetectContentType(data)
				if !strings.HasPrefix(mime, "image/") {
					mime = "image/jpeg"
				}
				return data, mime
			}
		}
		if u.StickerMessage != nil {
			if webp, err := c.wa.Download(context.Background(), u.StickerMessage); err == nil && len(webp) > 0 {
				id := fmt.Sprintf("%d", time.Now().UnixNano())
				inPath := filepath.Join(c.cfg.RootDir, "temp", "vis_"+id+".webp")
				outPath := filepath.Join(c.cfg.RootDir, "temp", "vis_"+id+".png")
				defer os.Remove(inPath)
				defer os.Remove(outPath)
				if os.WriteFile(inPath, webp, 0644) == nil {
					cmd := exec.Command("ffmpeg", "-y", "-i", inPath, "-frames:v", "1", outPath)
					if cmd.Run() == nil {
						if png, err := os.ReadFile(outPath); err == nil && len(png) > 0 {
							return png, "image/png"
						}
					}
				}
				return webp, "image/webp"
			}
		}
		return nil, ""
	}

	if data, mime := tryMsg(evt.Message); len(data) > 0 {
		return data, mime
	}
	if ci := getMsgContextInfo(evt.Message); ci != nil && ci.QuotedMessage != nil {
		if data, mime := tryMsg(ci.QuotedMessage); len(data) > 0 {
			return data, mime
		}
	}
	return nil, ""
}

func (c *Client) replyAsLunar(evt *events.Message, senderName, body string, isDono bool) {
	chatID := evt.Info.Chat.String()
	if senderName == "" {
		senderName = "amigo"
	}

	imgData, imgMime := c.extractMediaForVision(evt)
	if body == "" {
		if len(imgData) > 0 {
			body = "(enviou uma figurinha ou imagem, reaja ao que você vê nela)"
		} else {
			body = "oi"
		}
	}

	wantsAudio := audioRequestRE.MatchString(body)
	if wantsAudio {
		_ = c.wa.SendChatPresence(context.Background(), evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
	} else {
		_ = c.wa.SendChatPresence(context.Background(), evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	}

	var reply string
	if c.cfg.GeminiAPIKey != "" {
		if resp, err := c.askLunarGemini(chatID, senderName, body, isDono, imgData, imgMime); err == nil && resp != "" {
			reply = resp
		} else if err != nil {
			logger.Yellow("[Lunar Persona] Gemini fallback: %v", err)
		}
	}

	if reply == "" {
		reply = lunarFallbackReply(senderName, body, isDono, c.cfg.NomeDoDono)
	}

	if wantsAudio {
		if ogg, err := SynthesizeVoice(c.cfg.RootDir, c.cfg.GeminiAPIKey, reply); err == nil && len(ogg) > 0 {
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

func (c *Client) askLunarGemini(chatID, senderName, body string, isDono bool, imgData []byte, imgMime string) (string, error) {
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
		"Se mandarem imagem ou figurinha, comente diretamente sobre o que você vê nela (meme, texto, expressão, zoeira) como uma pessoa reagindo no zap. " +
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
	userParts := []map[string]any{
		{"text": userMsg},
	}
	if len(imgData) > 0 && imgMime != "" {
		userParts = append(userParts, map[string]any{
			"inlineData": map[string]string{
				"mimeType": imgMime,
				"data":     base64.StdEncoding.EncodeToString(imgData),
			},
		})
	}
	contents = append(contents, map[string]any{
		"role":  "user",
		"parts": userParts,
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

	var lastErr error
	for _, model := range lunarModels {
		endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, url.QueryEscape(c.cfg.GeminiAPIKey))
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := personaHTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s HTTP %d: %s", model, resp.StatusCode, string(respBytes[:min(len(respBytes), 180)]))
			continue
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
			lastErr = err
			continue
		}
		if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
			lastErr = fmt.Errorf("%s empty candidates", model)
			continue
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
			return text, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("empty gemini response")
	}
	return "", lastErr
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

package bot

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func findEdgeTTS() string {
	if p, err := exec.LookPath("edge-tts"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	cand := filepath.Join(home, ".local", "bin", "edge-tts")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return "edge-tts"
}

// SynthesizeVoice generates a WhatsApp-ready OGG/Opus PTT voice note.
// Prefers Gemini 2.5 Flash TTS when apiKey is configured, falling back to edge-tts.
func SynthesizeVoice(rootDir, apiKey, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("texto vazio")
	}
	if apiKey != "" {
		voice := os.Getenv("GEMINI_VOICE")
		if voice == "" {
			voice = "Zephyr"
		}
		if ogg, err := synthesizeGeminiTTS(rootDir, apiKey, voice, text); err == nil && len(ogg) > 0 {
			return ogg, nil
		}
	}
	return synthesizeEdgeTTS(rootDir, text)
}

func synthesizeGeminiTTS(rootDir, apiKey, voice, text string) ([]byte, error) {
	payload := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": "Fale em português brasileiro de forma natural e expressiva: " + text}}},
		},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig": map[string]any{
				"voiceConfig": map[string]any{
					"prebuiltVoiceConfig": map[string]string{
						"voiceName": voice,
					},
				},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-preview-tts:generateContent?key=" + url.QueryEscape(apiKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 15<<20))
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini tts empty response")
	}
	b64 := parsed.Candidates[0].Content.Parts[0].InlineData.Data
	if b64 == "" {
		return nil, fmt.Errorf("gemini tts missing audio data")
	}
	pcm, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	pcmPath := filepath.Join(rootDir, "temp", "gtts_"+id+".pcm")
	oggPath := filepath.Join(rootDir, "temp", "gtts_"+id+".ogg")
	defer os.Remove(pcmPath)
	defer os.Remove(oggPath)

	if err := os.WriteFile(pcmPath, pcm, 0644); err != nil {
		return nil, err
	}
	ff := exec.Command("ffmpeg", "-y", "-f", "s16le", "-ar", "24000", "-ac", "1", "-i", pcmPath, "-c:a", "libopus", "-b:a", "48k", "-ac", "1", "-ar", "48000", oggPath)
	if out, err := ff.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg pcm->ogg: %v (%s)", err, string(out))
	}
	return os.ReadFile(oggPath)
}

func synthesizeEdgeTTS(rootDir, text string) ([]byte, error) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	mp3Path := filepath.Join(rootDir, "temp", "tts_"+id+".mp3")
	oggPath := filepath.Join(rootDir, "temp", "tts_"+id+".ogg")
	defer os.Remove(mp3Path)
	defer os.Remove(oggPath)

	cmd := exec.Command(findEdgeTTS(),
		"--voice", "pt-BR-FranciscaNeural",
		"--rate=+4%",
		"--text", text,
		"--write-media", mp3Path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("edge-tts: %v (%s)", err, string(out))
	}

	ff := exec.Command("ffmpeg", "-y", "-i", mp3Path, "-c:a", "libopus", "-b:a", "48k", "-ac", "1", "-ar", "48000", oggPath)
	if out, err := ff.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg opus: %v (%s)", err, string(out))
	}
	return os.ReadFile(oggPath)
}

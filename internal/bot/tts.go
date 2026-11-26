package bot

import (
	"fmt"
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

// SynthesizeVoice generates a WhatsApp-ready OGG/Opus PTT voice note using edge-tts + ffmpeg.
func SynthesizeVoice(rootDir, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("texto vazio")
	}
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

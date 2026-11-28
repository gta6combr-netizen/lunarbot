package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type MoedaConfig struct {
	Nome          string `json:"nome"`
	Simbolo       string `json:"simbolo"`
	DailyMin      int64  `json:"dailyMin"`
	DailyMax      int64  `json:"dailyMax"`
	XpPorMensagem int64  `json:"xpPorMensagem"`
	XpPorComando  int64  `json:"xpPorComando"`
}

type NiveisConfig struct {
	XpBase        int64   `json:"xpBase"`
	Multiplicador float64 `json:"multiplicador"`
}

type Config struct {
	Prefix             string       `json:"prefix"`
	NomeDoBot          string       `json:"NomeDoBot"`
	NomeDoDono         string       `json:"NomeDoDono"`
	NumeroDoDono       string       `json:"NumeroDoDono"`
	Packname           string       `json:"packname"`
	Author             string       `json:"author"`
	CooldownPadrao     int          `json:"cooldownPadrao"`
	TempCleanupMinutes int          `json:"tempCleanupMinutes"`
	GeminiAPIKey       string       `json:"geminiApiKey"`
	RapidAPIKey        string       `json:"rapidApiKey"`
	ElevenLabsAPIKey   string       `json:"elevenLabsApiKey"`
	ElevenLabsVoiceID  string       `json:"elevenLabsVoiceId"`
	GiphyAPIKey        string       `json:"giphyApiKey"`
	KlipyAPIKey        string       `json:"klipyApiKey"`
	Moeda              MoedaConfig  `json:"moeda"`
	Niveis             NiveisConfig `json:"niveis"`
	RootDir            string       `json:"-"`
}

func Load(rootDir string) (*Config, error) {
	cfg := &Config{
		Prefix:             ".",
		NomeDoBot:          "Lunar Bot",
		NomeDoDono:         "LCSX",
		Packname:           "BY LCSX",
		Author:             "BY LCSX",
		CooldownPadrao:     3,
		TempCleanupMinutes: 30,
		Moeda: MoedaConfig{
			Nome:          "LunarCoins",
			Simbolo:       "🌙",
			DailyMin:      50,
			DailyMax:      150,
			XpPorMensagem: 5,
			XpPorComando:  10,
		},
		Niveis: NiveisConfig{
			XpBase:        100,
			Multiplicador: 1.35,
		},
		RootDir: rootDir,
	}

	cfgPath := filepath.Join(rootDir, "config", "config.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	if v := os.Getenv("OWNER_NUMBER"); v != "" {
		cfg.NumeroDoDono = v
	}
	if v := os.Getenv("BOT_NAME"); v != "" {
		cfg.NomeDoBot = v
	}
	if v := os.Getenv("OWNER_NAME"); v != "" {
		cfg.NomeDoDono = v
	}
	if v := os.Getenv("API_GEMINI"); v != "" {
		cfg.GeminiAPIKey = v
	} else if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		cfg.GeminiAPIKey = v
	}
	if v := os.Getenv("RAPIDAPI_KEY"); v != "" {
		cfg.RapidAPIKey = v
	}
	if v := os.Getenv("ELEVENLABS_API_KEY"); v != "" {
		cfg.ElevenLabsAPIKey = v
	}
	if v := os.Getenv("ELEVENLABS_VOICE_ID"); v != "" {
		cfg.ElevenLabsVoiceID = v
	}
	if cfg.ElevenLabsVoiceID == "" {
		cfg.ElevenLabsVoiceID = "NS7yU4WmAouJsg84X3Mg"
	}
	if v := os.Getenv("GIPHY_API_KEY"); v != "" {
		cfg.GiphyAPIKey = v
	}
	if v := os.Getenv("KLIPY_API_KEY"); v != "" {
		cfg.KlipyAPIKey = v
	}

	if cfg.Prefix == "" {
		cfg.Prefix = "."
	}
	if cfg.CooldownPadrao <= 0 {
		cfg.CooldownPadrao = 3
	}
	if cfg.TempCleanupMinutes <= 0 {
		cfg.TempCleanupMinutes = 30
	}
	if cfg.Niveis.XpBase <= 0 {
		cfg.Niveis.XpBase = 100
	}
	if cfg.Niveis.Multiplicador <= 1.0 {
		cfg.Niveis.Multiplicador = 1.35
	}

	return cfg, nil
}

func CleanNumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

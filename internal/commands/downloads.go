package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ytURLIDRE    = regexp.MustCompile(`(?:v=|youtu\.be/|shorts/|embed/)([a-zA-Z0-9_-]{11})`)
	ytSearchIDRE = regexp.MustCompile(`"videoId":"([a-zA-Z0-9_-]{11})"`)
)

func extractOrSearchYouTubeID(query string) (string, error) {
	if m := ytURLIDRE.FindStringSubmatch(query); len(m) == 2 {
		return m[1], nil
	}
	u := "https://www.youtube.com/results?search_query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if m := ytSearchIDRE.FindSubmatch(body); len(m) == 2 {
		return string(m[1]), nil
	}
	return "", fmt.Errorf("vídeo não encontrado")
}

func downloadRapidAPIMP3(apiKey, videoID string) ([]byte, error) {
	const host = "youtube-mp3-audio-video-downloader.p.rapidapi.com"
	endpoint := fmt.Sprintf("https://%s/download-mp3/%s", host, url.PathEscape(videoID))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-RapidAPI-Key", apiKey)
	req.Header.Set("X-RapidAPI-Host", host)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	cl := &http.Client{Timeout: 60 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("rapidapi HTTP %d: %s", resp.StatusCode, string(b))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 30<<20))
	if err != nil {
		return nil, err
	}
	if len(data) < 1024 {
		return nil, fmt.Errorf("arquivo de áudio inválido")
	}
	return data, nil
}

func findYtDlp() string {
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	cand := filepath.Join(home, ".local", "bin", "yt-dlp")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return "yt-dlp"
}

func registerDownloads(r *Registry) {
	r.Register(&Command{
		Name: "play", Aliases: []string{"ytmp3", "musica"}, Category: "downloads",
		Description: "Baixa áudio do YouTube pelo nome ou link",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o nome da música ou link.")
			}
			ctx.React("🎵")
			_ = ctx.Reply("⏳ Baixando áudio...")

			if ctx.Config.RapidAPIKey != "" {
				if vid, err := extractOrSearchYouTubeID(ctx.Q); err == nil && vid != "" {
					if data, err := downloadRapidAPIMP3(ctx.Config.RapidAPIKey, vid); err == nil {
						return ctx.SendAudio(data, false)
					}
				}
			}

			query := ctx.Q
			if !strings.HasPrefix(query, "http") {
				query = "ytsearch1:" + query
			}
			outTmpl := filepath.Join(ctx.Config.RootDir, "temp", fmt.Sprintf("play_%d.%%(ext)s", time.Now().UnixNano()))
			cmd := exec.Command(findYtDlp(), "-x", "--audio-format", "mp3", "--max-filesize", "25M", "-o", outTmpl, query)
			if out, err := cmd.CombinedOutput(); err != nil {
				return ctx.Reply("❌ Erro ao baixar: " + string(out))
			}

			pattern := strings.Replace(outTmpl, "%(ext)s", "*", 1)
			matches, _ := filepath.Glob(pattern)
			if len(matches) == 0 {
				return ctx.Reply("❌ Arquivo de áudio não encontrado.")
			}
			defer os.Remove(matches[0])
			data, err := os.ReadFile(matches[0])
			if err != nil {
				return ctx.Reply("❌ Erro ao ler arquivo.")
			}
			return ctx.SendAudio(data, false)
		},
	})

	r.Register(&Command{
		Name: "ytmp4", Aliases: []string{"video"}, Category: "downloads",
		Description: "Baixa vídeo do YouTube",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o nome ou link do vídeo.")
			}
			ctx.React("🎬")
			query := ctx.Q
			if !strings.HasPrefix(query, "http") {
				query = "ytsearch1:" + query
			}
			outPath := filepath.Join(ctx.Config.RootDir, "temp", fmt.Sprintf("vid_%d.mp4", time.Now().UnixNano()))
			defer os.Remove(outPath)
			cmd := exec.Command(findYtDlp(), "-f", "mp4[filesize<30M]/best[ext=mp4]/best", "--max-filesize", "30M", "-o", outPath, query)
			if out, err := cmd.CombinedOutput(); err != nil {
				return ctx.Reply("❌ Erro ao baixar vídeo: " + string(out))
			}
			data, err := os.ReadFile(outPath)
			if err != nil {
				return ctx.Reply("❌ Erro ao ler vídeo.")
			}
			return ctx.SendVideo(data, "🎬 "+ctx.Q, false)
		},
	})

	r.Register(&Command{
		Name: "ytsearch", Aliases: []string{"yts"}, Category: "downloads",
		Description: "Pesquisa vídeos no YouTube",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o que deseja pesquisar.")
			}
			cmd := exec.Command(findYtDlp(), "--flat-playlist", "--print", "%(title)s | https://youtu.be/%(id)s", "ytsearch5:"+ctx.Q)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return ctx.Reply("❌ Erro na pesquisa: " + err.Error())
			}
			return ctx.Reply("🔎 *Resultados YouTube:*\n\n" + strings.TrimSpace(string(out)))
		},
	})

	videoDownloader := func(name, desc string) {
		r.Register(&Command{
			Name: name, Category: "downloads", Description: desc,
			Execute: func(ctx *Context) error {
				if ctx.Q == "" || !strings.HasPrefix(ctx.Q, "http") {
					return ctx.Reply("❗ Informe uma URL válida.")
				}
				ctx.React("📥")
				outPath := filepath.Join(ctx.Config.RootDir, "temp", fmt.Sprintf("%s_%d.mp4", name, time.Now().UnixNano()))
				defer os.Remove(outPath)
				cmd := exec.Command(findYtDlp(), "-f", "mp4/best", "--max-filesize", "30M", "-o", outPath, ctx.Q)
				if out, err := cmd.CombinedOutput(); err != nil {
					return ctx.Reply("❌ Erro ao baixar: " + string(out))
				}
				data, err := os.ReadFile(outPath)
				if err != nil {
					return ctx.Reply("❌ Erro ao ler vídeo.")
				}
				return ctx.SendVideo(data, "✅ Download concluído!", false)
			},
		})
	}
	videoDownloader("tiktok", "Baixa vídeo do TikTok sem marca d'água")
	videoDownloader("instagram", "Baixa Reels/vídeo do Instagram")

	r.Register(&Command{
		Name: "pinterest", Category: "downloads", Description: "Busca imagens no Pinterest",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o termo de busca.")
			}
			return ctx.Reply("📌 Busca Pinterest: https://www.pinterest.com/search/pins/?q=" + strings.ReplaceAll(ctx.Q, " ", "%20"))
		},
	})
}

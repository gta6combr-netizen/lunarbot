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
	"strconv"
	"strings"
	"time"
)

type ytMeta struct {
	Title       string
	Author      string
	Duration    string
	Description string
	ThumbURL    string
}

func fetchYouTubeMeta(videoID string) ytMeta {
	meta := ytMeta{
		ThumbURL: fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", videoID),
	}

	// 1. Fast oEmbed for title & author (~150ms)
	var oe struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
		ThumbURL   string `json:"thumbnail_url"`
	}
	if err := httpGetJSON("https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v="+url.QueryEscape(videoID)+"&format=json", &oe); err == nil {
		meta.Title = oe.Title
		meta.Author = oe.AuthorName
		if oe.ThumbURL != "" {
			meta.ThumbURL = oe.ThumbURL
		}
	}

	// 2. Fast youtubei/v1/player for duration & description
	ytBody := map[string]any{
		"videoId": videoID,
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "ANDROID",
				"clientVersion": "19.09.37",
				"hl":            "pt",
				"gl":            "BR",
			},
		},
	}
	var pr struct {
		VideoDetails struct {
			Title            string `json:"title"`
			Author           string `json:"author"`
			LengthSeconds    string `json:"lengthSeconds"`
			ShortDescription string `json:"shortDescription"`
		} `json:"videoDetails"`
	}
	if httpPostJSON("https://www.youtube.com/youtubei/v1/player", ytBody, nil, &pr) == nil {
		if meta.Title == "" {
			meta.Title = pr.VideoDetails.Title
		}
		if meta.Author == "" {
			meta.Author = pr.VideoDetails.Author
		}
		if pr.VideoDetails.ShortDescription != "" {
			meta.Description = pr.VideoDetails.ShortDescription
		}
		if sec, err := strconv.Atoi(pr.VideoDetails.LengthSeconds); err == nil && sec > 0 {
			meta.Duration = fmt.Sprintf("%d:%02d", sec/60, sec%60)
		}
	}

	if meta.Title == "" {
		meta.Title = "Áudio do YouTube"
	}
	if len([]rune(meta.Description)) > 350 {
		r := []rune(meta.Description)
		meta.Description = string(r[:350]) + "..."
	}
	return meta
}

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

	cl := &http.Client{Timeout: 90 * time.Second}
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

			vid, err := extractOrSearchYouTubeID(ctx.Q)
			if err == nil && vid != "" {
				type audioRes struct {
					data []byte
					err  error
				}
				audioCh := make(chan audioRes, 1)
				if ctx.Config.RapidAPIKey != "" {
					go func() {
						d, e := downloadRapidAPIMP3(ctx.Config.RapidAPIKey, vid)
						audioCh <- audioRes{d, e}
					}()
				} else {
					audioCh <- audioRes{nil, fmt.Errorf("no rapidapi key")}
				}

				meta := fetchYouTubeMeta(vid)
				thumb, _ := httpGetBytes(meta.ThumbURL)
				ytURL := "https://youtu.be/" + vid

				var capSB strings.Builder
				capSB.WriteString(fmt.Sprintf("🎵 *%s*\n", meta.Title))
				if meta.Author != "" {
					capSB.WriteString(fmt.Sprintf("👤 *Canal:* %s\n", meta.Author))
				}
				if meta.Duration != "" {
					capSB.WriteString(fmt.Sprintf("⏱️ *Duração:* %s\n", meta.Duration))
				}
				capSB.WriteString(fmt.Sprintf("🔗 %s\n", ytURL))
				if meta.Description != "" {
					capSB.WriteString(fmt.Sprintf("\n📝 *Descrição:*\n%s\n", meta.Description))
				}

				if len(thumb) > 0 {
					_ = ctx.SendImage(thumb, capSB.String())
				} else {
					_ = ctx.Reply(capSB.String())
				}

				res := <-audioCh
				if res.err == nil && len(res.data) > 0 {
					return ctx.SendAudio(res.data, false)
				}
			}

			query := ctx.Q
			if !strings.HasPrefix(query, "http") {
				query = "ytsearch1:" + query
			}
			outTmpl := filepath.Join(ctx.Config.RootDir, "temp", fmt.Sprintf("play_%d.%%(ext)s", time.Now().UnixNano()))
			cmd := exec.Command(findYtDlp(), "--extractor-args", "youtube:player_client=android", "-x", "--audio-format", "mp3", "--max-filesize", "25M", "-o", outTmpl, query)
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

	socialDownloader := func(name string, aliases []string, desc string) {
		r.Register(&Command{
			Name: name, Aliases: aliases, Category: "downloads", Description: desc,
			Execute: func(ctx *Context) error {
				if ctx.Q == "" || !strings.HasPrefix(ctx.Q, "http") {
					return ctx.Reply("❗ Informe uma URL válida.")
				}
				ctx.React("📥")
				if ctx.Config.RapidAPIKey != "" {
					if data, isImg, err := downloadRapidSocialMedia(ctx.Config.RapidAPIKey, ctx.Q); err == nil && len(data) > 512 {
						if isImg {
							return ctx.SendImage(data, "✅ Download concluído!")
						}
						return ctx.SendVideo(data, "✅ Download concluído!", false)
					}
				}
				data, err := downloadVideoFromURL(ctx, ctx.Q)
				if err != nil {
					return ctx.Reply("❌ Erro ao baixar: " + err.Error())
				}
				return ctx.SendVideo(data, "✅ Download concluído!", false)
			},
		})
	}
	socialDownloader("tiktok", []string{"tt"}, "Baixa vídeo/foto do TikTok sem marca d'água")
	socialDownloader("instagram", []string{"ig", "insta", "reels"}, "Baixa Reels/post/story do Instagram")
	socialDownloader("facebook", []string{"fb"}, "Baixa vídeo/Reels do Facebook")
	socialDownloader("x", []string{"twitter", "tw"}, "Baixa vídeo/GIF/foto do X (Twitter)")
	socialDownloader("kwai", []string{"kw"}, "Baixa vídeo do Kwai")
	socialDownloader("baixar", []string{"dl", "alldl", "vimeo", "reddit", "twitch", "dailymotion"}, "Baixa vídeo/mídia de qualquer link suportado")

	r.Register(&Command{
		Name: "pinterest", Aliases: []string{"pin"}, Category: "downloads",
		Description: "Baixa vídeo/imagem de link do Pinterest ou busca pins",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe um link do Pinterest ou termo de busca.")
			}
			if strings.HasPrefix(ctx.Q, "http") {
				ctx.React("📥")
				if ctx.Config.RapidAPIKey != "" {
					if data, isImg, err := downloadRapidSocialMedia(ctx.Config.RapidAPIKey, ctx.Q); err == nil && len(data) > 512 {
						if isImg {
							return ctx.SendImage(data, "📌 Pinterest")
						}
						return ctx.SendVideo(data, "📌 Pinterest", false)
					}
				}
				data, err := downloadVideoFromURL(ctx, ctx.Q)
				if err != nil {
					return ctx.Reply("❌ Erro ao baixar: " + err.Error())
				}
				return ctx.SendVideo(data, "📌 Pinterest", false)
			}
			return ctx.Reply("📌 Busca Pinterest: https://www.pinterest.com/search/pins/?q=" + strings.ReplaceAll(ctx.Q, " ", "%20"))
		},
	})
}

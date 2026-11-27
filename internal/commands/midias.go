package commands

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io"
	"mime/multipart"
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
	cropDetectRE = regexp.MustCompile(`crop=\d+:\d+:\d+:\d+`)
	httpURLRE    = regexp.MustCompile(`https?://[^\s]+`)
)

func isSocialMediaURL(rawURL string) bool {
	l := strings.ToLower(rawURL)
	for _, d := range []string{
		"instagram.com", "tiktok.com", "facebook.com", "fb.watch", "fb.com",
		"x.com", "twitter.com", "pinterest.", "pin.it", "kwai.com",
		"vimeo.com", "twitch.tv", "dailymotion.com", "reddit.com", "snapchat.com", "linkedin.com", "bilibili.com",
	} {
		if strings.Contains(l, d) {
			return true
		}
	}
	return false
}

func looksLikeImage(data []byte, mediaType string) bool {
	if strings.EqualFold(mediaType, "photo") || strings.EqualFold(mediaType, "image") {
		return true
	}
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true // JPEG
	}
	if len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return true // PNG
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true // WEBP
	}
	return false
}

// downloadRapidAllInOne calls RapidAPI "Download All in One!" (Instagram, TikTok, Facebook, X, Pinterest).
func downloadRapidAllInOne(apiKey, rawURL string) ([]byte, bool, error) {
	if apiKey == "" {
		return nil, false, fmt.Errorf("rapidapi key missing")
	}
	const host = "instagram-downloader-download-instagram-videos-stories1.p.rapidapi.com"
	var endpoints []string
	if custom := strings.TrimSpace(os.Getenv("RAPIDAPI_SOCIAL_ENDPOINT")); custom != "" {
		if strings.HasPrefix(custom, "http") {
			endpoints = append(endpoints, custom+"?url="+url.QueryEscape(rawURL))
		} else {
			if !strings.HasPrefix(custom, "/") {
				custom = "/" + custom
			}
			endpoints = append(endpoints, "https://"+host+custom+"?url="+url.QueryEscape(rawURL))
		}
	}
	for _, p := range []string{"/download", "/index", "/v1/download", "/"} {
		endpoints = append(endpoints, "https://"+host+p+"?url="+url.QueryEscape(rawURL))
	}

	headers := map[string]string{
		"X-RapidAPI-Key":  apiKey,
		"X-RapidAPI-Host": host,
		"User-Agent":      "Mozilla/5.0",
	}

	var lastErr error
	for _, ep := range endpoints {
		b, err := httpGetWithHeaders(ep, headers)
		if err != nil {
			lastErr = err
			continue
		}
		var resp struct {
			DownloadURL string `json:"download_url"`
			URL         string `json:"url"`
			Type        string `json:"type"`
			Items       []struct {
				DownloadURL string `json:"download_url"`
				URL         string `json:"url"`
				Type        string `json:"type"`
			} `json:"items"`
		}
		if err := jsonUnmarshal(b, &resp); err != nil {
			lastErr = err
			continue
		}
		mediaURL := resp.DownloadURL
		mediaType := resp.Type
		if mediaURL == "" {
			mediaURL = resp.URL
		}
		if mediaURL == "" && len(resp.Items) > 0 {
			mediaURL = resp.Items[0].DownloadURL
			if mediaURL == "" {
				mediaURL = resp.Items[0].URL
			}
			if resp.Items[0].Type != "" {
				mediaType = resp.Items[0].Type
			}
		}
		if !strings.HasPrefix(mediaURL, "http") {
			lastErr = fmt.Errorf("download_url vazio")
			continue
		}
		data, err := httpGetBytes(mediaURL)
		if err != nil || len(data) < 512 {
			lastErr = fmt.Errorf("erro ao baixar mídia: %v", err)
			continue
		}
		return data, looksLikeImage(data, mediaType), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("falha na RapidAPI")
	}
	return nil, false, lastErr
}

func extractFirstMediaURL(b []byte) string {
	var anyJSON any
	if err := jsonUnmarshal(b, &anyJSON); err != nil {
		return ""
	}
	var fallback string
	var walk func(v any) string
	walk = func(v any) string {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range []string{"download_url", "video_url", "video", "play", "hd", "sd", "url", "image", "img", "src"} {
				if val, ok := x[k]; ok {
					if s, ok := val.(string); ok && strings.HasPrefix(s, "http") {
						sl := strings.ToLower(s)
						if strings.Contains(sl, ".mp4") || k == "download_url" || k == "video_url" || k == "video" || k == "play" || k == "hd" {
							return s
						}
						if fallback == "" && !strings.Contains(sl, "thumb") && !strings.Contains(sl, "avatar") {
							fallback = s
						}
					}
				}
			}
			for _, k := range []string{"items", "medias", "formats", "links", "data", "result"} {
				if val, ok := x[k]; ok {
					if hit := walk(val); hit != "" {
						return hit
					}
				}
			}
			for _, val := range x {
				if hit := walk(val); hit != "" {
					return hit
				}
			}
		case []any:
			for _, item := range x {
				if hit := walk(item); hit != "" {
					return hit
				}
			}
		case string:
			if strings.HasPrefix(x, "http") && fallback == "" {
				fallback = x
			}
		}
		return ""
	}
	if hit := walk(anyJSON); hit != "" {
		return hit
	}
	return fallback
}

func rapidGetMedia(host string, paths []string, apiKey, rawURL string) ([]byte, bool, error) {
	headers := map[string]string{
		"X-RapidAPI-Key":  apiKey,
		"X-RapidAPI-Host": host,
		"User-Agent":      "Mozilla/5.0",
	}
	var lastErr error
	for _, p := range paths {
		ep := "https://" + host + p + "?url=" + url.QueryEscape(rawURL)
		b, err := httpGetWithHeaders(ep, headers)
		if err != nil {
			lastErr = err
			continue
		}
		mediaURL := extractFirstMediaURL(b)
		if mediaURL == "" {
			lastErr = fmt.Errorf("mídia não encontrada na resposta")
			continue
		}
		data, err := httpGetBytes(mediaURL)
		if err != nil || len(data) < 512 {
			lastErr = fmt.Errorf("erro ao baixar mídia: %v", err)
			continue
		}
		return data, looksLikeImage(data, ""), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("falha na RapidAPI (%s)", host)
	}
	return nil, false, lastErr
}

func downloadRapidPinterest(apiKey, rawURL string) ([]byte, bool, error) {
	return rapidGetMedia("pinterest-video-and-image-downloader.p.rapidapi.com", []string{"/", "/pinterest", "/download", "/media"}, apiKey, rawURL)
}

func downloadRapidKwai(apiKey, rawURL string) ([]byte, bool, error) {
	return rapidGetMedia("kwai-downloader1.p.rapidapi.com", []string{"/", "/kwai", "/download", "/get-kwai-data"}, apiKey, rawURL)
}

func downloadRapidAllVideo(apiKey, rawURL string) ([]byte, bool, error) {
	const host = "all-video-downloader1.p.rapidapi.com"
	var lastErr error
	for _, p := range []string{"/all", "/download", "/"} {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		_ = w.WriteField("url", rawURL)
		_ = w.Close()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://"+host+p, &body)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("X-RapidAPI-Key", apiKey)
		req.Header.Set("X-RapidAPI-Host", host)
		req.Header.Set("User-Agent", "Mozilla/5.0")

		cl := &http.Client{Timeout: 30 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		mediaURL := extractFirstMediaURL(b)
		if mediaURL == "" {
			lastErr = fmt.Errorf("mídia não encontrada")
			continue
		}
		data, err := httpGetBytes(mediaURL)
		if err != nil || len(data) < 512 {
			lastErr = fmt.Errorf("erro ao baixar mídia: %v", err)
			continue
		}
		return data, looksLikeImage(data, ""), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("falha na All Video Downloader")
	}
	return nil, false, lastErr
}

func downloadRapidSocialMedia(apiKey, rawURL string) ([]byte, bool, error) {
	if apiKey == "" {
		return nil, false, fmt.Errorf("rapidapi key missing")
	}
	l := strings.ToLower(rawURL)
	if strings.Contains(l, "pinterest.") || strings.Contains(l, "pin.it") {
		if d, img, err := downloadRapidPinterest(apiKey, rawURL); err == nil {
			return d, img, nil
		}
		if d, img, err := downloadRapidAllInOne(apiKey, rawURL); err == nil {
			return d, img, nil
		}
		return downloadRapidAllVideo(apiKey, rawURL)
	}
	if strings.Contains(l, "kwai.com") {
		if d, img, err := downloadRapidKwai(apiKey, rawURL); err == nil {
			return d, img, nil
		}
		return downloadRapidAllVideo(apiKey, rawURL)
	}
	if d, img, err := downloadRapidAllInOne(apiKey, rawURL); err == nil {
		return d, img, nil
	}
	return downloadRapidAllVideo(apiKey, rawURL)
}

func downloadVideoFromURL(ctx *Context, rawURL string) ([]byte, error) {
	// 0. RapidAPI social downloaders (All-in-One, Pinterest, Kwai, All Video)
	if ctx.Config.RapidAPIKey != "" && isSocialMediaURL(rawURL) {
		if data, isImg, err := downloadRapidSocialMedia(ctx.Config.RapidAPIKey, rawURL); err == nil && !isImg && len(data) > 1024 {
			return data, nil
		}
	}

	// 1. TikTok: try RapidAPI (if subscribed) then tikwm.com (free, no watermark)
	if strings.Contains(strings.ToLower(rawURL), "tiktok.com") {
		if ctx.Config.RapidAPIKey != "" {
			var rapi struct {
				Video    string `json:"video"`
				Play     string `json:"play"`
				Download string `json:"download_url"`
				Data     struct {
					Play string `json:"play"`
				} `json:"data"`
			}
			ep := "https://tiktok-video-downloader25.p.rapidapi.com/download%20video?url=" + url.QueryEscape(rawURL)
			if b, err := httpGetWithHeaders(ep, map[string]string{
				"X-RapidAPI-Key":  ctx.Config.RapidAPIKey,
				"X-RapidAPI-Host": "tiktok-video-downloader25.p.rapidapi.com",
				"User-Agent":      "Mozilla/5.0",
			}); err == nil && len(b) > 0 {
				if strings.HasPrefix(string(b[:min(len(b), 12)]), "\x00\x00\x00") {
					return b, nil
				}
				if err := jsonUnmarshal(b, &rapi); err == nil {
					for _, u := range []string{rapi.Video, rapi.Play, rapi.Download, rapi.Data.Play} {
						if strings.HasPrefix(u, "http") {
							if vid, err := httpGetBytes(u); err == nil && len(vid) > 1024 {
								return vid, nil
							}
						}
					}
				}
			}
		}

		var tw struct {
			Code int `json:"code"`
			Data struct {
				Play string `json:"play"`
			} `json:"data"`
		}
		if err := httpGetJSON("https://www.tikwm.com/api/?url="+url.QueryEscape(rawURL), &tw); err == nil && tw.Code == 0 && tw.Data.Play != "" {
			if vid, err := httpGetBytes(tw.Data.Play); err == nil && len(vid) > 1024 {
				return vid, nil
			}
		}
	}

	// 2. Fallback: yt-dlp (YouTube, Instagram, TikTok, etc.)
	outPath := filepath.Join(ctx.Config.RootDir, "temp", fmt.Sprintf("dlvid_%d.mp4", time.Now().UnixNano()))
	defer os.Remove(outPath)
	ytdlp := "yt-dlp"
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		ytdlp = p
	} else if home, err := os.UserHomeDir(); err == nil {
		cand := filepath.Join(home, ".local", "bin", "yt-dlp")
		if _, err := os.Stat(cand); err == nil {
			ytdlp = cand
		}
	}
	cmd := exec.Command(ytdlp, "-f", "mp4[filesize<25M]/best[ext=mp4]/best", "--max-filesize", "25M", "-o", outPath, rawURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%v (%s)", err, string(out))
	}
	return os.ReadFile(outPath)
}

// detectCrop runs ffmpeg cropdetect to strip baked-in black bars (e.g. TikTok/Reels letterboxing).
func detectCrop(ctx *Context, inData []byte, ext string) string {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	inPath := filepath.Join(ctx.Config.RootDir, "temp", "cd_"+id+"."+ext)
	defer os.Remove(inPath)
	if os.WriteFile(inPath, inData, 0644) != nil {
		return ""
	}
	cmd := exec.Command("ffmpeg", "-y", "-i", inPath, "-t", "1.5", "-vf", "cropdetect=24:2:0", "-f", "null", "-")
	out, _ := cmd.CombinedOutput()
	matches := cropDetectRE.FindAllString(string(out), -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func downloadContextMedia(ctx *Context) ([]byte, string, error) {
	// Check own message first, then quoted
	candidates := []*Context{ctx}
	_ = candidates
	m := ctx.Message
	if m.ImageMessage != nil {
		b, err := ctx.Bot.Raw().Download(context.Background(), m.ImageMessage)
		return b, "image", err
	}
	if m.VideoMessage != nil {
		b, err := ctx.Bot.Raw().Download(context.Background(), m.VideoMessage)
		return b, "video", err
	}
	if qm := getQuotedMessage(ctx); qm != nil {
		inner := unwrapViewOnce(qm)
		if inner.ImageMessage != nil {
			b, err := ctx.Bot.Raw().Download(context.Background(), inner.ImageMessage)
			return b, "image", err
		}
		if inner.VideoMessage != nil {
			b, err := ctx.Bot.Raw().Download(context.Background(), inner.VideoMessage)
			return b, "video", err
		}
		if inner.AudioMessage != nil {
			b, err := ctx.Bot.Raw().Download(context.Background(), inner.AudioMessage)
			return b, "audio", err
		}
		if inner.StickerMessage != nil {
			b, err := ctx.Bot.Raw().Download(context.Background(), inner.StickerMessage)
			return b, "sticker", err
		}
	}
	return nil, "", fmt.Errorf("nenhuma mídia encontrada")
}

func runFFmpeg(ctx *Context, inData []byte, inExt, outExt string, args []string) ([]byte, error) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	inPath := filepath.Join(ctx.Config.RootDir, "temp", "in_"+id+"."+inExt)
	outPath := filepath.Join(ctx.Config.RootDir, "temp", "out_"+id+"."+outExt)
	defer os.Remove(inPath)
	defer os.Remove(outPath)

	if err := os.WriteFile(inPath, inData, 0644); err != nil {
		return nil, err
	}
	fullArgs := append([]string{"-y", "-i", inPath}, args...)
	fullArgs = append(fullArgs, outPath)
	cmd := exec.Command("ffmpeg", fullArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v (%s)", err, string(out))
	}
	return os.ReadFile(outPath)
}

func registerMidias(r *Registry) {
	r.Register(&Command{
		Name: "sticker", Aliases: []string{"s", "fig", "figurinha", "stickerwm"}, Category: "midias",
		Description: "Cria figurinha de imagem ou vídeo",
		Execute: func(ctx *Context) error {
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || (kind != "image" && kind != "video") {
				return ctx.Reply("❗ Envie ou responda uma imagem/vídeo.")
			}
			ctx.React("✨")
			ext := "jpg"
			if kind == "video" {
				ext = "mp4"
			}
			vf := "scale=512:512,setsar=1"
			if crop := detectCrop(ctx, data, ext); crop != "" {
				vf = crop + "," + vf
			}
			if kind == "video" {
				vf = "fps=12," + vf
			}
			ffArgs := []string{"-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-compression_level", "1", "-q:v", "65", "-loop", "0", "-an"}
			if kind == "video" {
				ffArgs = append([]string{"-t", "7"}, ffArgs...)
			}
			webp, err := runFFmpeg(ctx, data, ext, "webp", ffArgs)
			if err != nil {
				return ctx.Reply("❌ Erro ao gerar figurinha: " + err.Error())
			}
			return ctx.SendSticker(webp)
		},
	})

	r.Register(&Command{
		Name: "stickercirculo", Category: "midias", Description: "Cria figurinha circular",
		Execute: func(ctx *Context) error {
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || kind != "image" {
				return ctx.Reply("❗ Envie ou responda uma imagem.")
			}
			webp, err := runFFmpeg(ctx, data, "jpg", "webp", []string{
				"-vf", "scale=512:512:force_original_aspect_ratio=increase,crop=512:512,format=yuva420p,geq=lum='p(X,Y)':a='if(lte((X-256)^2+(Y-256)^2,256^2),255,0)'",
				"-vcodec", "libwebp",
			})
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.SendSticker(webp)
		},
	})

	r.Register(&Command{
		Name: "stickertexto", Category: "midias", Description: "Cria figurinha com texto (.stickertexto Olá)",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o texto.")
			}
			safe := strings.ReplaceAll(ctx.Q, "'", "")
			id := fmt.Sprintf("%d", time.Now().UnixNano())
			outPath := filepath.Join(ctx.Config.RootDir, "temp", "stxt_"+id+".webp")
			defer os.Remove(outPath)
			cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=black:s=512x512:d=1",
				"-vf", fmt.Sprintf("drawtext=text='%s':fontcolor=white:fontsize=48:x=(w-text_w)/2:y=(h-text_h)/2", safe),
				"-frames:v", "1", "-vcodec", "libwebp", outPath)
			if out, err := cmd.CombinedOutput(); err != nil {
				return ctx.Reply("❌ Erro: " + string(out))
			}
			webp, err := os.ReadFile(outPath)
			if err != nil {
				return ctx.Reply("❌ Erro ao ler figurinha.")
			}
			return ctx.SendSticker(webp)
		},
	})

	r.Register(&Command{
		Name: "tomp3", Category: "midias", Description: "Converte vídeo respondido em áudio MP3",
		Execute: func(ctx *Context) error {
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || kind != "video" {
				return ctx.Reply("❗ Responda a um vídeo.")
			}
			mp3, err := runFFmpeg(ctx, data, "mp4", "mp3", []string{"-vn", "-ab", "192k"})
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.SendAudio(mp3, false)
		},
	})

	r.Register(&Command{
		Name: "acelerar", Category: "midias", Description: "Acelera áudio/vídeo em 1.5x",
		Execute: func(ctx *Context) error {
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || (kind != "audio" && kind != "video") {
				return ctx.Reply("❗ Responda um áudio ou vídeo.")
			}
			if kind == "audio" {
				out, err := runFFmpeg(ctx, data, "ogg", "mp3", []string{"-filter:a", "atempo=1.5"})
				if err != nil {
					return ctx.Reply("❌ Erro: " + err.Error())
				}
				return ctx.SendAudio(out, true)
			}
			out, err := runFFmpeg(ctx, data, "mp4", "mp4", []string{"-filter_complex", "[0:v]setpts=0.67*PTS[v];[0:a]atempo=1.5[a]", "-map", "[v]", "-map", "[a]"})
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.SendVideo(out, "⚡ Acelerado 1.5x", false)
		},
	})

	r.Register(&Command{
		Name: "volume", Category: "midias", Description: "Aumenta o volume do áudio",
		Execute: func(ctx *Context) error {
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || kind != "audio" {
				return ctx.Reply("❗ Responda um áudio.")
			}
			out, err := runFFmpeg(ctx, data, "ogg", "mp3", []string{"-filter:a", "volume=3.0"})
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.SendAudio(out, true)
		},
	})

	// Image filter commands using ffmpeg
	imgFilter := func(name, desc, vf string) {
		r.Register(&Command{
			Name: name, Category: "midias", Description: desc,
			Execute: func(ctx *Context) error {
				data, kind, err := downloadContextMedia(ctx)
				if err != nil || kind != "image" {
					return ctx.Reply("❗ Envie ou responda uma imagem.")
				}
				out, err := runFFmpeg(ctx, data, "jpg", "jpg", []string{"-vf", vf})
				if err != nil {
					return ctx.Reply("❌ Erro: " + err.Error())
				}
				return ctx.SendImage(out, "")
			},
		})
	}
	imgFilter("pb", "Converte imagem para preto e branco", "format=gray")
	imgFilter("invert", "Inverte as cores da imagem", "negate")
	imgFilter("blur", "Aplica desfoque na imagem", "boxblur=10:1")
	imgFilter("espelhar", "Espelha a imagem horizontalmente", "hflip")
	imgFilter("girar", "Gira a imagem 90 graus", "transpose=1")

	r.Register(&Command{
		Name: "meme", Category: "midias", Description: "Adiciona legenda de meme na imagem (.meme Topo | Baixo)",
		Execute: func(ctx *Context) error {
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || kind != "image" {
				return ctx.Reply("❗ Envie ou responda uma imagem.")
			}
			parts := strings.SplitN(ctx.Q, "|", 2)
			top := strings.ReplaceAll(strings.TrimSpace(parts[0]), "'", "")
			bot := ""
			if len(parts) > 1 {
				bot = strings.ReplaceAll(strings.TrimSpace(parts[1]), "'", "")
			}
			vf := fmt.Sprintf("drawtext=text='%s':fontcolor=white:borderw=3:fontsize=40:x=(w-text_w)/2:y=20,drawtext=text='%s':fontcolor=white:borderw=3:fontsize=40:x=(w-text_w)/2:y=h-th-20", top, bot)
			out, err := runFFmpeg(ctx, data, "jpg", "jpg", []string{"-vf", vf})
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.SendImage(out, "")
		},
	})

	r.Register(&Command{
		Name: "removebg", Category: "midias", Description: "Remove fundo (requer API externa)",
		Execute: func(ctx *Context) error {
			return ctx.Reply("⚠️ Configure `REMOVEBG_API_KEY` para usar remoção de fundo.")
		},
	})

	r.Register(&Command{
		Name: "toanime", Category: "midias", Description: "Aplica filtro estilizado na imagem",
		Execute: func(ctx *Context) error {
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || kind != "image" {
				return ctx.Reply("❗ Envie ou responda uma imagem.")
			}
			out, err := runFFmpeg(ctx, data, "jpg", "jpg", []string{"-vf", "eq=saturation=1.8:contrast=1.2,edgedetect=mode=colormix"})
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.SendImage(out, "🎨 Estilizado!")
		},
	})

	r.Register(&Command{
		Name:        "figaudio",
		Aliases:     []string{"stickeraudio", "saudio", "figurinhaaudio", "scomaudio"},
		Category:    "midias",
		Description: "Cria figurinha animada + áudio a partir de vídeo ou áudio respondido",
		Execute: func(ctx *Context) error {
			var data []byte
			var kind string
			var err error

			if u := httpURLRE.FindString(ctx.Q); u != "" {
				ctx.React("📥")
				_ = ctx.Reply("⬇️ Baixando vídeo do link para fazer figurinha...")
				data, err = downloadVideoFromURL(ctx, u)
				if err != nil {
					return ctx.Reply("❌ Erro ao baixar vídeo do link: " + err.Error())
				}
				kind = "video"
			} else {
				data, kind, err = downloadContextMedia(ctx)
				if err != nil || (kind != "video" && kind != "audio") {
					return ctx.Reply("❗ Responda a um *vídeo/áudio* ou mande um link (ex: `.figaudio https://vm.tiktok.com/...`).")
				}
			}
			ctx.React("🔊")

			if kind == "video" {
				vf := "scale=512:512,setsar=1"
				if crop := detectCrop(ctx, data, "mp4"); crop != "" {
					vf = crop + "," + vf
				}
				// Tenta até 20s mantendo <=480KB para o WhatsApp não congelar a animação
				vf20 := "fps=10,scale=320:320,setsar=1"
				if crop := detectCrop(ctx, data, "mp4"); crop != "" {
					vf20 = crop + "," + vf20
				}
				webp, err := runFFmpeg(ctx, data, "mp4", "webp", []string{
					"-t", "20",
					"-vf", vf20,
					"-vcodec", "libwebp", "-lossless", "0", "-compression_level", "2", "-q:v", "40", "-loop", "0", "-an",
				})
				if err != nil {
					return ctx.Reply("❌ Erro ao gerar figurinha: " + err.Error())
				}
				if len(webp) > 480*1024 {
					vfSmall := "fps=8,scale=256:256,setsar=1"
					if crop := detectCrop(ctx, data, "mp4"); crop != "" {
						vfSmall = crop + "," + vfSmall
					}
					if smaller, err := runFFmpeg(ctx, data, "mp4", "webp", []string{
						"-t", "20",
						"-vf", vfSmall,
						"-vcodec", "libwebp", "-lossless", "0", "-compression_level", "3", "-q:v", "30", "-loop", "0", "-an",
					}); err == nil && len(smaller) > 0 {
						webp = smaller
					}
				}
				ogg, err := runFFmpeg(ctx, data, "mp4", "ogg", []string{"-t", "30", "-vn", "-c:a", "libopus", "-b:a", "48k", "-ac", "1", "-ar", "48000"})
				if err != nil {
					_ = ctx.SendSticker(webp)
					return ctx.Reply("⚠️ Figurinha enviada sem áudio (o vídeo não possui faixa de áudio válida).")
				}
				return ctx.SendStickerWithAudio(webp, ogg, true)
			}

			// kind == "audio": gera figurinha animada com waveform + áudio embutido no mesmo StickerMessage
			webp, err := runFFmpeg(ctx, data, "ogg", "webp", []string{
				"-t", "6",
				"-filter_complex", "[0:a]showwaves=s=512x512:mode=cline:rate=15:colors=0xa855f7|0x38bdf8,format=yuva420p[v]",
				"-map", "[v]",
				"-vcodec", "libwebp", "-lossless", "0", "-q:v", "70", "-loop", "0", "-vsync", "0",
			})
			if err != nil {
				return ctx.Reply("❌ Erro ao gerar figurinha do áudio: " + err.Error())
			}
			ogg, err := runFFmpeg(ctx, data, "ogg", "ogg", []string{"-t", "30", "-c:a", "libopus", "-b:a", "48k", "-ac", "1", "-ar", "48000"})
			if err != nil {
				ogg = data
			}
			return ctx.SendStickerWithAudio(webp, ogg, true)
		},
	})

	r.Register(&Command{
		Name:        "fala",
		Aliases:     []string{"tts", "audio", "voz"},
		Category:    "midias",
		Description: "Faz a Lunar falar um texto em áudio (PTT)",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Escreve o que você quer que eu fale! Ex: `.fala oi gente`")
			}
			ogg, err := ctx.Bot.SynthesizeVoice(ctx.Q)
			if err != nil {
				return ctx.Reply("❌ Erro ao gerar áudio: " + err.Error())
			}
			return ctx.SendAudio(ogg, true)
		},
	})

	r.Register(&Command{
		Name: "ttp", Aliases: []string{"attp", "figtexto", "sticktexto"}, Category: "midias",
		Description: "Cria figurinha de texto animada letra por letra (.sticktexto <texto>)",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Escreve o texto da figurinha! Ex: `.sticktexto Aaaaaa`")
			}
			ctx.React("✨")
			bg := color.RGBA{0x56, 0x82, 0x03, 255} // #568203
			fg := color.RGBA{255, 255, 255, 255}
			frames, err := renderTextStickerFrames(ctx.Config.RootDir, ctx.Q, bg, fg)
			if err != nil || len(frames) == 0 {
				return ctx.Reply("❌ Erro ao gerar texto.")
			}

			frameDir := filepath.Join(ctx.Config.RootDir, "temp", fmt.Sprintf("ttp_%d", time.Now().UnixNano()))
			if err := os.MkdirAll(frameDir, 0755); err != nil {
				return ctx.Reply("❌ Erro ao criar pasta temporária.")
			}
			defer os.RemoveAll(frameDir)

			for i, fBytes := range frames {
				p := filepath.Join(frameDir, fmt.Sprintf("f_%03d.png", i))
				if err := os.WriteFile(p, fBytes, 0644); err != nil {
					return ctx.Reply("❌ Erro ao salvar frame.")
				}
			}
			outWebp := filepath.Join(frameDir, "out.webp")
			cmd := exec.Command("ffmpeg", "-y", "-framerate", "10", "-i", filepath.Join(frameDir, "f_%03d.png"),
				"-vf", "scale=512:512,setsar=1", "-vcodec", "libwebp", "-lossless", "0", "-q:v", "80", "-loop", "0", "-an", outWebp)
			if out, err := cmd.CombinedOutput(); err != nil {
				return ctx.Reply("❌ Erro ao animar figurinha: " + string(out))
			}
			webp, err := os.ReadFile(outWebp)
			if err != nil || len(webp) == 0 {
				return ctx.Reply("❌ Erro ao ler figurinha.")
			}
			return ctx.SendSticker(webp)
		},
	})
}

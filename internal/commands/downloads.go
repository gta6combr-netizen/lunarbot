package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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

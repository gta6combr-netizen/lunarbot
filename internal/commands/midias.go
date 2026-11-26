package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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
			ffArgs := []string{"-vf", "format=rgba,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=0x00000000", "-vcodec", "libwebp", "-lossless", "0", "-q:v", "75", "-loop", "0", "-an", "-vsync", "0"}
			if kind == "video" {
				ext = "mp4"
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
			data, kind, err := downloadContextMedia(ctx)
			if err != nil || (kind != "video" && kind != "audio") {
				return ctx.Reply("❗ Responda a um *vídeo* ou *áudio* para criar figurinha com áudio.")
			}
			ctx.React("🔊")

			if kind == "video" {
				webp, err := runFFmpeg(ctx, data, "mp4", "webp", []string{
					"-t", "7",
					"-vf", "format=rgba,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=0x00000000",
					"-vcodec", "libwebp", "-lossless", "0", "-q:v", "75", "-loop", "0", "-an", "-vsync", "0",
				})
				if err != nil {
					return ctx.Reply("❌ Erro ao gerar figurinha: " + err.Error())
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
}

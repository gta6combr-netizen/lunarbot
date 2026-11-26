package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type gifReactionSpec struct {
	Name    string
	Aliases []string
	Emoji   string
	Verb    string
	GifFile string
	WaifuEP string
}

type meterSpec struct {
	Name    string
	Aliases []string
	Emoji   string
	Label   string
}

func registerResenha(r *Registry) {
	reactions := []gifReactionSpec{
		{Name: "beijo", Aliases: []string{"kiss", "beijar"}, Emoji: "💋", Verb: "beijou", GifFile: "beijo.gif", WaifuEP: "kiss"},
		{Name: "abraco", Aliases: []string{"abracar", "abracarforte", "hug"}, Emoji: "🤗", Verb: "abraçou", GifFile: "abracar.gif", WaifuEP: "hug"},
		{Name: "tapa", Aliases: []string{"slap", "tapear"}, Emoji: "👋", Verb: "deu um tapa em", GifFile: "tapa.gif", WaifuEP: "slap"},
		{Name: "soco", Aliases: []string{"socar", "punch"}, Emoji: "👊", Verb: "deu um soco em", GifFile: "soco.gif", WaifuEP: "kick"},
		{Name: "chute", Aliases: []string{"chutar", "kick_fun"}, Emoji: "🦵", Verb: "chutou", GifFile: "chute.gif", WaifuEP: "kick"},
		{Name: "mordida", Aliases: []string{"morder", "bite"}, Emoji: "😈", Verb: "mordeu", GifFile: "mordida.gif", WaifuEP: "bite"},
		{Name: "carinho", Aliases: []string{"acariciar", "pat", "cafune"}, Emoji: "🥰", Verb: "fez carinho em", GifFile: "carinho.gif", WaifuEP: "pat"},
		{Name: "lambida", Aliases: []string{"lamber", "lick"}, Emoji: "👅", Verb: "lambeu", GifFile: "lambida.gif", WaifuEP: "lick"},
		{Name: "apertao", Aliases: []string{"apertar", "cuddle"}, Emoji: "🫂", Verb: "apertou", GifFile: "apertao.gif", WaifuEP: "cuddle"},
		{Name: "cafunecabeca", Aliases: []string{"headpat"}, Emoji: "💆", Verb: "fez cafuné em", GifFile: "carinho_cabeca.gif", WaifuEP: "pat"},
		{Name: "matar", Aliases: []string{"kill"}, Emoji: "🔪", Verb: "eliminou", WaifuEP: "kill"},
		{Name: "cutucar", Aliases: []string{"poke"}, Emoji: "👉", Verb: "cutucou", WaifuEP: "poke"},
		{Name: "piscar", Aliases: []string{"wink"}, Emoji: "😉", Verb: "piscou para", WaifuEP: "wink"},
		{Name: "corar", Aliases: []string{"blush"}, Emoji: "😳", Verb: "corou por causa de", WaifuEP: "blush"},
		{Name: "assustar", Aliases: []string{"bully"}, Emoji: "👻", Verb: "assustou", WaifuEP: "bully"},
	}

	for _, rx := range reactions {
		spec := rx
		r.Register(&Command{
			Name: spec.Name, Aliases: spec.Aliases, Category: "resenha",
			Description: fmt.Sprintf("Reação: %s alguém", spec.Verb),
			Execute: func(ctx *Context) error {
				target := resolveTarget(ctx)
				if target.IsEmpty() {
					return ctx.Reply(fmt.Sprintf("❗ Marque ou responda alguém.\nEx: *%s%s @pessoa*", ctx.Prefix, spec.Name))
				}
				ctx.React(spec.Emoji)
				caption := fmt.Sprintf("%s %s *%s* %s!", spec.Emoji, mentionTag(ctx.Sender), spec.Verb, mentionTag(target))

				// Try local GIF -> MP4 via ffmpeg
				if spec.GifFile != "" {
					gifPath := filepath.Join(ctx.Config.RootDir, "assets", "gifs", spec.GifFile)
					if gifBytes, err := os.ReadFile(gifPath); err == nil {
						if mp4, err := runFFmpeg(ctx, gifBytes, "gif", "mp4", []string{"-movflags", "faststart", "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2"}); err == nil {
							return ctx.SendVideo(mp4, caption, true, ctx.Sender.String(), target.String())
						}
					}
				}
				return ctx.Bot.SendText(ctx.From, caption, ctx.Sender.String(), target.String())
			},
		})
	}

	meters := []meterSpec{
		{Name: "gado", Aliases: []string{"gadometro"}, Emoji: "🐂", Label: "gado"},
		{Name: "corno", Aliases: []string{"cornometro"}, Emoji: "🦌", Label: "corno"},
		{Name: "gay", Aliases: []string{"gaymetro"}, Emoji: "🏳️‍🌈", Label: "gay"},
		{Name: "lindo", Aliases: []string{"gostoso", "beleza"}, Emoji: "😍", Label: "lindo(a)"},
		{Name: "inteligente", Aliases: []string{"qi"}, Emoji: "🧠", Label: "inteligente"},
		{Name: "sortudo", Aliases: []string{"sorte"}, Emoji: "🍀", Label: "sortudo(a)"},
		{Name: "bravo", Aliases: []string{"raiva"}, Emoji: "😡", Label: "bravo(a)"},
		{Name: "nerd", Emoji: "🤓", Label: "nerd"},
		{Name: "boiola", Emoji: "💅", Label: "boiola"},
		{Name: "sigma", Emoji: "🗿", Label: "sigma"},
		{Name: "bobo", Aliases: []string{"tonto"}, Emoji: "🤡", Label: "palhaço"},
		{Name: "rico", Emoji: "💸", Label: "rico(a)"},
	}

	for _, m := range meters {
		ms := m
		r.Register(&Command{
			Name: ms.Name, Aliases: ms.Aliases, Category: "resenha",
			Description: fmt.Sprintf("Mede o nível de %s", ms.Label),
			Execute: func(ctx *Context) error {
				target := resolveTarget(ctx)
				if target.IsEmpty() {
					target = ctx.Sender
				}
				pct := randInt(0, 100)
				bar := strings.Repeat("█", pct/10) + strings.Repeat("░", 10-(pct/10))
				return ctx.Bot.SendText(ctx.From, fmt.Sprintf("%s %s é *%d%% %s*!\n[%s]", ms.Emoji, mentionTag(target), pct, ms.Label, bar), target.String())
			},
		})
	}

	r.Register(&Command{
		Name: "ship", Aliases: []string{"casal", "compatibilidade"}, Category: "resenha",
		Description: "Calcula compatibilidade entre duas pessoas",
		Execute: func(ctx *Context) error {
			t := resolveTarget(ctx)
			if t.IsEmpty() {
				return ctx.Reply("❗ Marque alguém para shippar!")
			}
			pct := randInt(0, 100)
			heart := "💔"
			if pct >= 75 {
				heart = "💘"
			} else if pct >= 45 {
				heart = "❤️"
			}
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("%s *SHIP*\n%s + %s = *%d%%*", heart, mentionTag(ctx.Sender), mentionTag(t), pct), ctx.Sender.String(), t.String())
		},
	})

	r.Register(&Command{
		Name: "8ball", Aliases: []string{"bola8", "vidente"}, Category: "resenha",
		Description: "Responde sim/não para uma pergunta",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("🎱 Faça uma pergunta!")
			}
			ans := []string{"Com certeza!", "Sim.", "Provavelmente.", "Talvez...", "Melhor não contar com isso.", "Não.", "Definitivamente não."}
			return ctx.Reply("🎱 " + ans[randInt(0, len(ans)-1)])
		},
	})

	r.Register(&Command{
		Name: "cantada", Category: "resenha", Description: "Manda uma cantada",
		Execute: func(ctx *Context) error {
			c := []string{
				"Você não é Wi-Fi, mas senti uma conexão imediata.",
				"Se beleza fosse bug, você travava a produção.",
				"Você tem mapa? Me perdi no seu sorriso.",
			}
			return ctx.Reply("💘 " + c[randInt(0, len(c)-1)])
		},
	})

	r.Register(&Command{
		Name: "topgado", Aliases: []string{"topcorno", "toprico", "topbonito"}, Category: "resenha", OnlyGroup: true,
		Description: "Ranking aleatório do grupo",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil || len(ctx.GroupInfo.Participants) == 0 {
				return ctx.Reply("❌ Sem participantes.")
			}
			parts := ctx.GroupInfo.Participants
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🏆 *TOP %s DO GRUPO*\n\n", strings.ToUpper(ctx.CmdName)))
			var mentions []string
			limit := 5
			if len(parts) < limit {
				limit = len(parts)
			}
			for i := 0; i < limit; i++ {
				p := parts[randInt(0, len(parts)-1)]
				mentions = append(mentions, p.JID.String())
				sb.WriteString(fmt.Sprintf("%d. %s — *%d%%*\n", i+1, mentionTag(p.JID), randInt(50, 100)))
			}
			return ctx.Bot.SendText(ctx.From, sb.String(), mentions...)
		},
	})
}

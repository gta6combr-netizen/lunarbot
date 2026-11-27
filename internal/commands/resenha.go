package commands

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"nyxbot-go/internal/config"
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

func validGIFBytes(b []byte) bool {
	return len(b) > 4096 && strings.HasPrefix(string(b[:6]), "GIF8")
}

func fetchGiphyGIF(apiKey, query string) []byte {
	if apiKey == "" || query == "" {
		return nil
	}
	ep := fmt.Sprintf("https://api.giphy.com/v1/gifs/search?api_key=%s&q=%s&limit=10&rating=pg-13",
		url.QueryEscape(apiKey), url.QueryEscape(query))
	var res struct {
		Data []struct {
			Images struct {
				Original struct {
					URL string `json:"url"`
				} `json:"original"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := httpGetJSON(ep, &res); err != nil || len(res.Data) == 0 {
		return nil
	}
	pick := res.Data[randInt(0, len(res.Data)-1)].Images.Original.URL
	if pick == "" {
		return nil
	}
	if b, err := httpGetBytes(pick); err == nil && validGIFBytes(b) {
		return b
	}
	return nil
}

func fetchKlipyGIF(apiKey, query string) []byte {
	if apiKey == "" || query == "" {
		return nil
	}
	// Klipy v1 API (and v2 fallback)
	v1 := fmt.Sprintf("https://api.klipy.com/api/v1/%s/gifs/search?q=%s&per_page=10",
		url.PathEscape(apiKey), url.QueryEscape(query))
	var res1 struct {
		Data struct {
			Data []struct {
				File struct {
					HD struct {
						GIF struct{ URL string `json:"url"` } `json:"gif"`
					} `json:"hd"`
					MD struct {
						GIF struct{ URL string `json:"url"` } `json:"gif"`
					} `json:"md"`
				} `json:"file"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := httpGetJSON(v1, &res1); err == nil && len(res1.Data.Data) > 0 {
		item := res1.Data.Data[randInt(0, len(res1.Data.Data)-1)]
		u := item.File.MD.GIF.URL
		if u == "" {
			u = item.File.HD.GIF.URL
		}
		if u != "" {
			if b, err := httpGetBytes(u); err == nil && validGIFBytes(b) {
				return b
			}
		}
	}
	v2 := fmt.Sprintf("https://api.klipy.com/v2/search?key=%s&q=%s&limit=10&media_filter=gif",
		url.QueryEscape(apiKey), url.QueryEscape(query))
	var res2 struct {
		Results []struct {
			MediaFormats struct {
				GIF struct{ URL string `json:"url"` } `json:"gif"`
			} `json:"media_formats"`
		} `json:"results"`
	}
	if err := httpGetJSON(v2, &res2); err == nil && len(res2.Results) > 0 {
		u := res2.Results[randInt(0, len(res2.Results)-1)].MediaFormats.GIF.URL
		if u != "" {
			if b, err := httpGetBytes(u); err == nil && validGIFBytes(b) {
				return b
			}
		}
	}
	return nil
}

func fetchReactionGIF(cfg *config.Config, ep string) []byte {
	if ep == "" {
		return nil
	}
	validNonGiphy := func(rawURL string, b []byte) bool {
		if strings.Contains(strings.ToLower(rawURL), "giphy.com") {
			return false
		}
		return validGIFBytes(b)
	}
	tryNekosBest := func() []byte {
		var nb struct {
			Results []struct {
				URL string `json:"url"`
			} `json:"results"`
		}
		if err := httpGetJSON("https://nekos.best/api/v2/"+ep, &nb); err == nil && len(nb.Results) > 0 && nb.Results[0].URL != "" {
			if b, err := httpGetBytes(nb.Results[0].URL); err == nil && validNonGiphy(nb.Results[0].URL, b) {
				return b
			}
		}
		return nil
	}
	tryKlipy := func() []byte {
		if cfg == nil || cfg.KlipyAPIKey == "" {
			return nil
		}
		return fetchKlipyGIF(cfg.KlipyAPIKey, "anime "+ep)
	}
	tryGiphy := func() []byte {
		if cfg == nil || cfg.GiphyAPIKey == "" {
			return nil
		}
		return fetchGiphyGIF(cfg.GiphyAPIKey, "anime "+ep)
	}
	tryWaifu := func() []byte {
		wep := ep
		if wep == "punch" {
			wep = "kick"
		}
		var wp struct {
			URL string `json:"url"`
		}
		if err := httpGetJSON("https://api.waifu.pics/sfw/"+wep, &wp); err == nil && wp.URL != "" {
			if b, err := httpGetBytes(wp.URL); err == nil && validNonGiphy(wp.URL, b) {
				return b
			}
		}
		return nil
	}
	tryOtakuGifs := func() []byte {
		var og struct {
			URL string `json:"url"`
		}
		if err := httpGetJSON("https://api.otakugifs.xyz/gif?reaction="+ep, &og); err == nil && og.URL != "" {
			if b, err := httpGetBytes(og.URL); err == nil && validNonGiphy(og.URL, b) {
				return b
			}
		}
		return nil
	}

	for _, fn := range []func() []byte{tryNekosBest, tryKlipy, tryGiphy, tryWaifu, tryOtakuGifs} {
		if b := fn(); len(b) > 0 {
			return b
		}
	}
	return nil
}

func registerResenha(r *Registry) {
	reactions := []gifReactionSpec{
		{Name: "beijo", Aliases: []string{"kiss", "beijar", "beijinho"}, Emoji: "💋", Verb: "beijou", GifFile: "beijo.gif", WaifuEP: "kiss"},
		{Name: "abraco", Aliases: []string{"abracar", "abraço", "abraçar", "abracarforte", "hug"}, Emoji: "🤗", Verb: "abraçou", GifFile: "abracar.gif", WaifuEP: "hug"},
		{Name: "tapa", Aliases: []string{"slap", "tapear"}, Emoji: "👋", Verb: "deu um tapa em", GifFile: "tapa.gif", WaifuEP: "slap"},
		{Name: "soco", Aliases: []string{"socar", "punch"}, Emoji: "👊", Verb: "deu um soco em", GifFile: "soco.gif", WaifuEP: "punch"},
		{Name: "chute", Aliases: []string{"chutar", "kick_fun"}, Emoji: "🦵", Verb: "chutou", GifFile: "chute.gif", WaifuEP: "kick"},
		{Name: "mordida", Aliases: []string{"morder", "bite"}, Emoji: "😈", Verb: "mordeu", GifFile: "mordida.gif", WaifuEP: "bite"},
		{Name: "carinho", Aliases: []string{"acariciar", "pat", "cafune", "cafuné"}, Emoji: "🥰", Verb: "fez carinho em", GifFile: "carinho.gif", WaifuEP: "pat"},
		{Name: "lambida", Aliases: []string{"lamber", "lick"}, Emoji: "👅", Verb: "lambeu", GifFile: "lambida.gif", WaifuEP: "lick"},
		{Name: "apertao", Aliases: []string{"apertar", "cuddle"}, Emoji: "🫂", Verb: "apertou", GifFile: "apertao.gif", WaifuEP: "cuddle"},
		{Name: "cafunecabeca", Aliases: []string{"headpat"}, Emoji: "💆", Verb: "fez cafuné em", GifFile: "carinho_cabeca.gif", WaifuEP: "pat"},
		{Name: "matar", Aliases: []string{"kill"}, Emoji: "🔪", Verb: "eliminou", WaifuEP: "kill"},
		{Name: "cutucar", Aliases: []string{"poke"}, Emoji: "👉", Verb: "cutucou", WaifuEP: "poke"},
		{Name: "piscar", Aliases: []string{"wink"}, Emoji: "😉", Verb: "piscou para", WaifuEP: "wink"},
		{Name: "corar", Aliases: []string{"blush"}, Emoji: "😳", Verb: "corou por causa de", WaifuEP: "blush"},
		{Name: "assustar", Aliases: []string{"bully"}, Emoji: "👻", Verb: "assustou", WaifuEP: "bully"},
		{Name: "dancar", Aliases: []string{"dançar", "dance", "danca"}, Emoji: "💃", Verb: "dançou com", WaifuEP: "dance"},
		{Name: "rir", Aliases: []string{"risada", "laugh"}, Emoji: "😂", Verb: "deu risada com", WaifuEP: "laugh"},
		{Name: "chorar", Aliases: []string{"choro", "cry"}, Emoji: "😭", Verb: "chorou no ombro de", WaifuEP: "cry"},
		{Name: "sorrir", Aliases: []string{"sorriso", "smile"}, Emoji: "😊", Verb: "sorriu para", WaifuEP: "smile"},
		{Name: "acenar", Aliases: []string{"wave"}, Emoji: "👋", Verb: "acenou para", WaifuEP: "wave"},
		{Name: "aplaudir", Aliases: []string{"palmas", "clap"}, Emoji: "👏", Verb: "aplaudiu", WaifuEP: "clap"},
		{Name: "tocaai", Aliases: []string{"highfive", "hi5"}, Emoji: "🙌", Verb: "deu um toca aqui em", WaifuEP: "highfive"},
		{Name: "segurar", Aliases: []string{"handhold", "maosdadas"}, Emoji: "🤝", Verb: "segurou a mão de", WaifuEP: "handhold"},
		{Name: "bonk", Aliases: []string{"martelo"}, Emoji: "🔨", Verb: "deu um bonk em", WaifuEP: "bonk"},
		{Name: "yeet", Aliases: []string{"arremessar"}, Emoji: "🚀", Verb: "arremessou", WaifuEP: "yeet"},
		{Name: "feliz", Aliases: []string{"happy", "alegre"}, Emoji: "✨", Verb: "ficou feliz por causa de", WaifuEP: "happy"},
		{Name: "encarar", Aliases: []string{"glare", "stare"}, Emoji: "👀", Verb: "encarou", WaifuEP: "glare"},
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

				// 1. Tenta API de GIFs (nekos.best / klipy / giphy / waifu.pics)
				gifBytes := fetchReactionGIF(ctx.Config, spec.WaifuEP)
				// 2. Fallback para GIF local se offline ou falhar
				if len(gifBytes) == 0 && spec.GifFile != "" {
					gifPath := filepath.Join(ctx.Config.RootDir, "assets", "gifs", spec.GifFile)
					gifBytes, _ = os.ReadFile(gifPath)
				}
				if len(gifBytes) > 0 {
					if mp4, err := runFFmpeg(ctx, gifBytes, "gif", "mp4", []string{"-movflags", "faststart", "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2"}); err == nil {
						return ctx.SendVideo(mp4, caption, true, ctx.Sender.String(), target.String())
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
				cap := fmt.Sprintf("%s %s é *%d%% %s*!\n[%s]", ms.Emoji, mentionTag(target), pct, ms.Label, bar)
				img, err := renderMeterCard("MEDIDOR: "+strings.ToUpper(ms.Label), "@"+target.User, pct, fmt.Sprintf("%s é %d%% %s!", target.User, pct, ms.Label), colPurple)
				return sendCard(ctx, img, err, cap, target.String())
			},
		})
	}

	r.Register(&Command{
		Name: "ship", Aliases: []string{"casal", "compatibilidade"}, Category: "resenha",
		Description: "Calcula compatibilidade entre duas pessoas",
		Execute: func(ctx *Context) error {
			t := resolveTarget(ctx)
			if t.IsEmpty() {
				return ctx.Reply("❗ Marca alguém pra eu shippar né, não tenho bola de cristal.")
			}
			pct := randInt(0, 100)
			heart := "💔"
			comment := "Esquece, nem com reza braba kkk"
			if pct >= 80 {
				heart = "💘"
				comment = "Já pode marcar o casamento, tô convidada!"
			} else if pct >= 55 {
				heart = "❤️"
				comment = "Rola química aí hein 👀"
			} else if pct >= 30 {
				heart = "😬"
				comment = "Meio torto, mas vai que..."
			}
			cap := fmt.Sprintf("%s *SHIP*\n%s + %s = *%d%%*\n%s", heart, mentionTag(ctx.Sender), mentionTag(t), pct, comment)
			img, err := renderVersusCard("MODO CASAL", "COMPATIBILIDADE", "@"+ctx.Sender.User, "@"+t.User, fmt.Sprintf("%d%%", pct), comment, fetchAvatar(ctx, ctx.Sender), fetchAvatar(ctx, t), colRose)
			return sendCard(ctx, img, err, cap, ctx.Sender.String(), t.String())
		},
	})

	r.Register(&Command{
		Name: "8ball", Aliases: []string{"bola8", "vidente"}, Category: "resenha",
		Description: "Responde sim/não para uma pergunta",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("🎱 Faz a pergunta direito ué!")
			}
			ans := []string{
				"Com certeza, vai fundo!",
				"Sim, confia.",
				"Provavelmente sim hein.",
				"Hmm, talvez... não bota a mão no fogo.",
				"Nem fudendo kkk",
				"Não, esquece isso.",
				"Olha... melhor não contar com isso não.",
			}
			chosen := ans[randInt(0, len(ans)-1)]
			img, err := renderGameCard("BOLA 8", ctx.Q, []string{"Resposta: " + chosen}, colPurple)
			return sendCard(ctx, img, err, "🎱 "+chosen)
		},
	})

	r.Register(&Command{
		Name: "cantada", Category: "resenha", Description: "Manda uma cantada (.cantada [@pessoa])",
		Execute: func(ctx *Context) error {
			c := []string{
				"Você não é Wi-Fi, mas senti uma conexão imediata aqui hein",
				"Se beleza fosse bug, você travava minha produção inteira.",
				"Não sou astrônoma, mas você é a coisa mais linda que já vi no céu hoje.",
				"Me chama de boleto e me paga atenção, vai.",
				"Você tem mapa? Me perdi feio no seu sorriso.",
				"Meu coração não tem banco de dados, mas já salvou você como favorito.",
				"Se amar você fosse crime, eu já tava cumprindo perpétua sem recorrer.",
				"Você não é eclipse lunar, mas para tudo só pra olhar pra você.",
			}
			chosen := c[randInt(0, len(c)-1)]
			target := resolveTarget(ctx)
			fromAv := fetchAvatar(ctx, ctx.Sender)
			if !target.IsEmpty() {
				toAv := fetchAvatar(ctx, target)
				img, err := renderCantadaCard(chosen, ctx.Sender.User, target.User, fromAv, toAv)
				cap := fmt.Sprintf("💘 %s para %s: *%s*", mentionTag(ctx.Sender), mentionTag(target), chosen)
				return sendCard(ctx, img, err, cap, ctx.Sender.String(), target.String())
			}
			img, err := renderCantadaCard(chosen, ctx.Sender.User, "", fromAv, nil)
			return sendCard(ctx, img, err, "💘 "+chosen)
		},
	})

	r.Register(&Command{
		Name: "verdade", Category: "resenha", Description: "Pergunta de Verdade ou Desafio",
		Execute: func(ctx *Context) error {
			qs := []string{
				"Qual foi a maior mentira que você já contou aqui no grupo?",
				"Quem desse grupo você pegaria se ninguém ficasse sabendo? 👀",
				"Já mandou mensagem pro(a) ex bêbado(a) e se arrependeu?",
				"Qual o seu maior segredo que ninguém aqui desconfia?",
				"Se tivesse que apagar alguém desse grupo agora, quem seria?",
			}
			chosen := qs[randInt(0, len(qs)-1)]
			img, err := renderGameCard("VERDADE", "Responda com sinceridade", []string{chosen}, colAmber)
			return sendCard(ctx, img, err, "🔥 *VERDADE:* "+chosen)
		},
	})

	r.Register(&Command{
		Name: "desafio", Category: "resenha", Description: "Desafio de Verdade ou Desafio",
		Execute: func(ctx *Context) error {
			ds := []string{
				"Manda um áudio cantando o refrão de uma música brega agora!",
				"Marca a 3ª pessoa da lista de membros e manda um 'tô com saudade' sem explicar.",
				"Troca sua foto de perfil por uma careta por 10 minutos.",
				"Manda o print da sua última conversa (pode borrar só o nome).",
				"Fala uma qualidade sincera de quem você menos conversa aqui.",
			}
			chosen := ds[randInt(0, len(ds)-1)]
			img, err := renderGameCard("DESAFIO", "Cumpra agora!", []string{chosen}, colCyan)
			return sendCard(ctx, img, err, "⚡ *DESAFIO:* "+chosen)
		},
	})

	r.Register(&Command{
		Name: "roletarussa", Aliases: []string{"roleta"}, Category: "resenha", Description: "Roleta russa no chat",
		Execute: func(ctx *Context) error {
			if randInt(1, 6) == 1 {
				u := ctx.DB.GetUser(ctx.Sender.String())
				loss := int64(30)
				if u.Coins < loss {
					loss = u.Coins
				}
				u.Coins -= loss
				u.Losses++
				_ = ctx.DB.SaveUser(u)
				cap := fmt.Sprintf("💥 *POW!* %s rodou e levou bala! Perdeu *%d %s* 💀", mentionTag(ctx.Sender), loss, ctx.Config.Moeda.Simbolo)
				img, err := renderGameCard("ROLETA RUSSA", "POW! Levou bala!", []string{fmt.Sprintf("@%s perdeu %d %s", ctx.Sender.User, loss, ctx.Config.Moeda.Simbolo)}, colRose)
				return sendCard(ctx, img, err, cap, ctx.Sender.String())
			}
			ctx.DB.AddCoins(ctx.Sender.String(), 25)
			cap := fmt.Sprintf("🔫 *Click...* %s sobreviveu e ganhou *+25 %s* 😎", mentionTag(ctx.Sender), ctx.Config.Moeda.Simbolo)
			img, err := renderGameCard("ROLETA RUSSA", "Click... Sobreviveu!", []string{fmt.Sprintf("@%s ganhou +25 %s", ctx.Sender.User, ctx.Config.Moeda.Simbolo)}, colEmerald)
			return sendCard(ctx, img, err, cap, ctx.Sender.String())
		},
	})

	r.Register(&Command{
		Name: "duelo", Aliases: []string{"x1"}, Category: "resenha", OnlyGroup: true,
		Description: "Desafia alguém pro X1 valendo LunarCoins",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() || target == ctx.Sender {
				return ctx.Reply("❗ Marca alguém pro X1! Ex: `.duelo @pessoa`")
			}
			winner, loser := ctx.Sender, target
			if randInt(0, 1) == 1 {
				winner, loser = target, ctx.Sender
			}
			uW := ctx.DB.GetUser(winner.String())
			uW.Wins++
			uW.Coins += 40
			_ = ctx.DB.SaveUser(uW)
			uL := ctx.DB.GetUser(loser.String())
			uL.Losses++
			_ = ctx.DB.SaveUser(uL)
			cap := fmt.Sprintf("⚔️ *X1 PESADO!*\n%s amassou %s na porrada e levou *+40 %s* 🏆", mentionTag(winner), mentionTag(loser), ctx.Config.Moeda.Simbolo)
			img, err := renderVersusCard("DUELO X1", "Vencedor: @"+winner.User, "@"+ctx.Sender.User, "@"+target.User, "VS", fmt.Sprintf("%s venceu e levou +40 %s!", winner.User, ctx.Config.Moeda.Simbolo), fetchAvatar(ctx, ctx.Sender), fetchAvatar(ctx, target), colAmber)
			return sendCard(ctx, img, err, cap, winner.String(), loser.String())
		},
	})

	r.Register(&Command{
		Name: "roubar", Category: "resenha", OnlyGroup: true, Description: "Tenta roubar LunarCoins de alguém",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() || target == ctx.Sender {
				return ctx.Reply("❗ Marca quem você quer assaltar kkk")
			}
			victim := ctx.DB.GetUser(target.String())
			if newInv, shielded := consumeInventoryItem(victim.Inventory, "Escudo"); shielded {
				victim.Inventory = newInv
				_ = ctx.DB.SaveUser(victim)
				cap := fmt.Sprintf("🛡️ %s tinha um *Escudo* da loja e bloqueou o assalto de %s!", mentionTag(target), mentionTag(ctx.Sender))
				img, err := renderGameCard("ASSALTO BLOQUEADO", "Escudo Ativado!", []string{fmt.Sprintf("@%s gastou 1 Escudo e protegeu suas moedas!", target.User)}, colCyan)
				return sendCard(ctx, img, err, cap, target.String(), ctx.Sender.String())
			}
			if victim.Coins < 20 {
				return ctx.Reply("💸 Essa pessoa tá lisa demais, nem vale a pena roubar.")
			}
			if randInt(1, 100) <= 45 {
				stolen := int64(randInt(15, 50))
				if stolen > victim.Coins {
					stolen = victim.Coins
				}
				victim.Coins -= stolen
				_ = ctx.DB.SaveUser(victim)
				ctx.DB.AddCoins(ctx.Sender.String(), stolen)
				cap := fmt.Sprintf("🥷 %s passou a mão em *%d %s* de %s e saiu correndo!", mentionTag(ctx.Sender), stolen, ctx.Config.Moeda.Simbolo, mentionTag(target))
				img, err := renderGameCard("ASSALTO", "Sucesso na fuga!", []string{fmt.Sprintf("%s levou %d %s de %s", ctx.Sender.User, stolen, ctx.Config.Moeda.Simbolo, target.User)}, colEmerald)
				return sendCard(ctx, img, err, cap, ctx.Sender.String(), target.String())
			}
			thief := ctx.DB.GetUser(ctx.Sender.String())
			fine := int64(25)
			if thief.Coins < fine {
				fine = thief.Coins
			}
			thief.Coins -= fine
			_ = ctx.DB.SaveUser(thief)
			cap := fmt.Sprintf("🚓 Deu ruim! %s foi pego tentando roubar %s e pagou *%d %s* de multa!", mentionTag(ctx.Sender), mentionTag(target), fine, ctx.Config.Moeda.Simbolo)
			img, err := renderGameCard("ASSALTO", "Preso em flagrante!", []string{fmt.Sprintf("%s pagou %d %s de multa", ctx.Sender.User, fine, ctx.Config.Moeda.Simbolo)}, colRose)
			return sendCard(ctx, img, err, cap, ctx.Sender.String(), target.String())
		},
	})

	r.Register(&Command{
		Name: "patada", Category: "resenha", Description: "Manda uma patada em alguém",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			ps := []string{
				"não sou espelho pra ficar refletindo tua frustração não 💅",
				"sua opinião é igual nota de 3 reais: ninguém aceita.",
				"tá carente? compra um hamster, não vem encher meu saco.",
				"fala mais baixo que tua relevância aqui é zero.",
			}
			p := ps[randInt(0, len(ps)-1)]
			img, err := renderGameCard("PATADA", "Toma essa!", []string{p}, colRose)
			if !target.IsEmpty() {
				return sendCard(ctx, img, err, fmt.Sprintf("🔥 %s, %s", mentionTag(target), p), target.String())
			}
			return sendCard(ctx, img, err, "🔥 "+p)
		},
	})

	r.Register(&Command{
		Name: "fofoca", Category: "resenha", OnlyGroup: true, Description: "Inventa uma fofoca aleatória do grupo",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil || len(ctx.GroupInfo.Participants) < 2 {
				return ctx.Reply("❌ Precisa de mais gente no grupo pra ter fofoca!")
			}
			parts := ctx.GroupInfo.Participants
			p1 := parts[randInt(0, len(parts)-1)].JID
			p2 := parts[randInt(0, len(parts)-1)].JID
			fofs := []struct{ cap, card string }{
				{
					fmt.Sprintf("☕ *PLANTÃO DA LUNAR:* Disseram que %s e %s tão trocando figurinha suspeita no PV 👀", mentionTag(p1), mentionTag(p2)),
					fmt.Sprintf("Disseram que @%s e @%s tão trocando figurinha suspeita no PV!", p1.User, p2.User),
				},
				{
					fmt.Sprintf("🚨 *FOFOCA:* %s falou mal do áudio de %s ontem à noite, eu não deixava!", mentionTag(p1), mentionTag(p2)),
					fmt.Sprintf("@%s falou mal do áudio de @%s ontem à noite!", p1.User, p2.User),
				},
				{
					fmt.Sprintf("🌙 *BABADO:* Vi %s stalkeando a foto de %s por 20 minutos...", mentionTag(p1), mentionTag(p2)),
					fmt.Sprintf("Vi @%s stalkeando a foto de @%s por 20 minutos...", p1.User, p2.User),
				},
			}
			pick := fofs[randInt(0, len(fofs)-1)]
			img, err := renderGameCard("PLANTÃO DA LUNAR", "Fofoca Quente", []string{pick.card}, colPurple)
			return sendCard(ctx, img, err, pick.cap, p1.String(), p2.String())
		},
	})

	r.Register(&Command{
		Name: "talarico", Category: "resenha", OnlyGroup: true, Description: "Aponta o talarico do grupo",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil || len(ctx.GroupInfo.Participants) == 0 {
				return ctx.Reply("❌ Sem participantes.")
			}
			p := ctx.GroupInfo.Participants[randInt(0, len(ctx.GroupInfo.Participants)-1)].JID
			cap := fmt.Sprintf("🐍 Alerta de talarico! Todo mundo escondendo o(a) crush de %s!", mentionTag(p))
			img, err := renderGameCard("ALERTA TALARICO", "@"+p.User, []string{"Todo mundo escondendo o(a) crush desse ser!"}, colEmerald)
			return sendCard(ctx, img, err, cap, p.String())
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
			var entries []rankEntry
			var mentions []string
			limit := 5
			if len(parts) < limit {
				limit = len(parts)
			}
			for i := 0; i < limit; i++ {
				p := parts[randInt(0, len(parts)-1)]
				pct := randInt(50, 100)
				mentions = append(mentions, p.JID.String())
				sb.WriteString(fmt.Sprintf("%d. %s — *%d%%*\n", i+1, mentionTag(p.JID), pct))
				entries = append(entries, rankEntry{
					Pos:    i + 1,
					Name:   "@" + p.JID.User,
					Stat:   fmt.Sprintf("%d%%", pct),
					Avatar: fetchAvatar(ctx, p.JID),
				})
			}
			img, err := renderRankCard("TOP "+strings.ToUpper(ctx.CmdName), entries, colAmber)
			return sendCard(ctx, img, err, sb.String(), mentions...)
		},
	})

	r.Register(&Command{
		Name: "gif", Aliases: []string{"giphy", "klipy"}, Category: "resenha",
		Description: "Busca um GIF no Klipy ou Giphy (.gif <termo>)",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o que buscar. Ex: `.gif gato`")
			}
			ctx.React("🎞️")
			gifBytes := fetchKlipyGIF(ctx.Config.KlipyAPIKey, ctx.Q)
			if len(gifBytes) == 0 {
				gifBytes = fetchGiphyGIF(ctx.Config.GiphyAPIKey, ctx.Q)
			}
			if len(gifBytes) == 0 {
				return ctx.Reply("❌ Nenhum GIF encontrado (ou configure `KLIPY_API_KEY` / `GIPHY_API_KEY`).")
			}
			mp4, err := runFFmpeg(ctx, gifBytes, "gif", "mp4", []string{"-movflags", "faststart", "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2"})
			if err != nil {
				return ctx.Reply("❌ Erro ao converter GIF.")
			}
			return ctx.SendVideo(mp4, "🎞️ "+ctx.Q, true)
		},
	})
}

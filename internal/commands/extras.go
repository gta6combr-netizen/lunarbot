package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func registerExtras(r *Registry) {
	// Saude
	r.Register(&Command{
		Name: "imc", Category: "saude", Description: "Calcula IMC (.imc peso altura)",
		Execute: func(ctx *Context) error {
			if len(ctx.Args) < 2 {
				return ctx.Reply("❗ Use: *.imc 70 1.75*")
			}
			peso, err1 := strconv.ParseFloat(strings.ReplaceAll(ctx.Args[0], ",", "."), 64)
			alt, err2 := strconv.ParseFloat(strings.ReplaceAll(ctx.Args[1], ",", "."), 64)
			if err1 != nil || err2 != nil || alt <= 0 {
				return ctx.Reply("❗ Valores inválidos.")
			}
			if alt > 3 {
				alt = alt / 100.0
			}
			imc := peso / (alt * alt)
			cat := "Peso normal"
			switch {
			case imc < 18.5:
				cat = "Abaixo do peso"
			case imc >= 25 && imc < 30:
				cat = "Sobrepeso"
			case imc >= 30:
				cat = "Obesidade"
			}
			return ctx.Reply(fmt.Sprintf("⚖️ *IMC:* %.2f (%s)", imc, cat))
		},
	})
	r.Register(&Command{
		Name: "agua", Category: "saude", Description: "Calcula meta diária de água (.agua peso)",
		Execute: func(ctx *Context) error {
			peso, err := strconv.ParseFloat(ctx.Q, 64)
			if err != nil || peso <= 0 {
				return ctx.Reply("❗ Informe seu peso em kg.")
			}
			litros := (peso * 35.0) / 1000.0
			return ctx.Reply(fmt.Sprintf("💧 Meta recomendada: *%.2f litros/dia*", litros))
		},
	})
	r.Register(&Command{
		Name: "dicasono", Category: "saude", Description: "Dica de higiene do sono",
		Execute: func(ctx *Context) error {
			return ctx.Reply("😴 Evite telas 45 minutos antes de dormir e mantenha o quarto escuro e fresco.")
		},
	})

	// Trabalho & Estudo & Financas & Infos
	r.Register(&Command{
		Name: "pomodoro", Category: "trabalho", Description: "Inicia timer Pomodoro (25min)",
		Execute: func(ctx *Context) error {
			mins := 25
			if len(ctx.Args) > 0 {
				if v, err := strconv.Atoi(ctx.Args[0]); err == nil && v > 0 && v <= 120 {
					mins = v
				}
			}
			go func() {
				time.Sleep(time.Duration(mins) * time.Minute)
				_ = ctx.Bot.SendText(ctx.From, fmt.Sprintf("🍅 *Pomodoro concluído, %s!* Hora de descansar.", mentionTag(ctx.Sender)), ctx.Sender.String())
			}()
			return ctx.Reply(fmt.Sprintf("🍅 Pomodoro iniciado por *%d minutos*!", mins))
		},
	})
	simpleTip := func(name, cat, desc, text string) {
		r.Register(&Command{Name: name, Category: cat, Description: desc, Execute: func(ctx *Context) error { return ctx.Reply(text) }})
	}
	simpleTip("carreira", "trabalho", "Dica de carreira", "💼 Construa portfólio público e documente resultados mensuráveis.")
	simpleTip("checklist", "trabalho", "Checklist diário", "✅ 1. Prioridade máxima\n✅ 2. Caixa de entrada zero\n✅ 3. Revisão de fim de dia")
	simpleTip("dicasreuniao", "trabalho", "Dicas para reuniões", "📋 Toda reunião precisa de pauta, dono e próximo passo claro.")
	simpleTip("emailpro", "trabalho", "Modelo de e-mail profissional", "📧 Assunto claro + contexto em 2 linhas + ação solicitada + prazo.")
	simpleTip("meta", "trabalho", "Estrutura de meta SMART", "🎯 Específica • Mensurável • Atingível • Relevante • Temporal.")
	simpleTip("motivacaotrabalho", "trabalho", "Motivação no trabalho", "🔥 Progresso diário pequeno supera picos raros de esforço.")
	simpleTip("produtividade", "trabalho", "Dica de produtividade", "⚡ Bloqueie 90 minutos sem notificações para trabalho profundo.")
	simpleTip("dicaestudo", "estudo", "Dica de estudo", "📚 Use repetição espaçada e active recall em vez de só reler.")
	simpleTip("resumo", "estudo", "Método Feynman", "📝 Explique o assunto em linguagem simples; onde travar, revise.")
	simpleTip("flashcard", "estudo", "Dica de flashcards", "🃏 1 conceito por cartão, pergunta curta na frente, resposta direta atrás.")
	simpleTip("juros", "financas", "Fórmula de juros compostos", "💰 M = C × (1 + i)^t — tempo é o maior multiplicador.")
	simpleTip("reserva", "financas", "Reserva de emergência", "🏦 Mantenha 6 meses de custo fixo em liquidez diária.")
	simpleTip("regra503020", "financas", "Regra 50/30/20", "📊 50% essenciais • 30% desejos • 20% investimentos.")
	simpleTip("botinfo", "infos", "Informações técnicas do bot", "🤖 *Lunar Bot Go* — Rewrite completo em Go usando `whatsmeow` + `SQLite`.")
	simpleTip("criador", "infos", "Créditos", "👑 Base original NyxBot V2 (LCSX) • Rewrite Go.")

	// Animais / Waifu APIs (cmds-aleatorios)
	imgAPI := func(name string, aliases []string, cat, desc, apiURL, field string) {
		r.Register(&Command{
			Name: name, Aliases: aliases, Category: cat, Description: desc,
			Execute: func(ctx *Context) error {
				var res map[string]any
				if err := httpGetJSON(apiURL, &res); err != nil {
					return ctx.Reply("❌ Erro na API.")
				}
				u, _ := res[field].(string)
				if u == "" {
					return ctx.Reply("❌ Imagem indisponível.")
				}
				b, err := httpGetBytes(u)
				if err != nil {
					return ctx.Reply("❌ Erro ao baixar imagem.")
				}
				return ctx.SendImage(b, "")
			},
		})
	}
	imgAPI("gato", []string{"cat", "gatinho"}, "cmds-aleatorios", "Foto aleatória de gato", "https://some-random-api.com/img/cat", "link")
	imgAPI("cachorro", []string{"dog", "doguinho"}, "cmds-aleatorios", "Foto de cachorro", "https://dog.ceo/api/breeds/image/random", "message")
	imgAPI("fox", nil, "cmds-aleatorios", "Foto de raposa", "https://randomfox.ca/floof/", "image")
	imgAPI("duck", nil, "cmds-aleatorios", "Foto de pato", "https://random-d.uk/api/v2/random", "url")
	imgAPI("bird", nil, "cmds-aleatorios", "Foto de pássaro", "https://some-random-api.com/img/bird", "link")
	imgAPI("koala", nil, "cmds-aleatorios", "Foto de coala", "https://some-random-api.com/img/koala", "link")
	imgAPI("kangaroo", nil, "cmds-aleatorios", "Foto de canguru", "https://some-random-api.com/img/kangaroo", "link")
	imgAPI("neko", nil, "cmds-aleatorios", "Imagem neko anime", "https://api.waifu.pics/sfw/neko", "url")
	imgAPI("waifu", nil, "cmds-aleatorios", "Imagem waifu anime", "https://api.waifu.pics/sfw/waifu", "url")
	imgAPI("hugimg", nil, "cmds-aleatorios", "Imagem de abraço anime", "https://api.waifu.pics/sfw/hug", "url")
	imgAPI("kissimg", nil, "cmds-aleatorios", "Imagem de beijo anime", "https://api.waifu.pics/sfw/kiss", "url")
	imgAPI("coffee", nil, "cmds-aleatorios", "Foto aleatória de café", "https://coffee.alexflipnote.dev/random.json", "file")
	imgAPI("panda", nil, "cmds-aleatorios", "Foto de panda", "https://some-random-api.com/img/panda", "link")

	// Casamento (Marriage)
	r.Register(&Command{
		Name: "casar", Category: "resenha", Description: "Pede alguém em casamento",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque outra pessoa para pedir em casamento.")
			}
			senderPN := ctx.Bot.ResolvePN(ctx.Sender).ToNonAD()
			targetPN := ctx.Bot.ResolvePN(target).ToNonAD()
			if senderPN.String() == targetPN.String() {
				return ctx.Reply("❗ Marque outra pessoa para pedir em casamento.")
			}
			u1 := ctx.DB.GetUser(senderPN.String())
			if u1.Spouse != "" {
				return ctx.Reply("💍 Você já está casado(a)!")
			}
			// If asking the bot itself in marriage:
			if ctx.Bot != nil && ctx.Bot.Raw() != nil && ctx.Bot.Raw().Store.ID != nil {
				botID := ctx.Bot.Raw().Store.ID.ToNonAD()
				botLID := ctx.Bot.Raw().Store.LID.ToNonAD()
				if targetPN.User == botID.User || target.User == botID.User || (!botLID.IsEmpty() && (targetPN.User == botLID.User || target.User == botLID.User)) {
					if !ctx.IsDono {
						return ctx.Reply("💅 Nem pensar, já tenho dono!")
					}
					now := time.Now().UnixMilli()
					botJIDStr := botID.String()
					uBot := ctx.DB.GetUser(botJIDStr)
					u1.Spouse = botJIDStr
					u1.MarriedAt = now
					uBot.Spouse = senderPN.String()
					uBot.MarriedAt = now
					_ = ctx.DB.SaveUser(u1)
					_ = ctx.DB.SaveUser(uBot)
					img, err := renderCasamentoCard(ctx.Config.RootDir, "@"+senderPN.User, "@"+botID.User, 100, fetchAvatar(ctx, senderPN), fetchAvatar(ctx, botID))
					cap := fmt.Sprintf("💖 *ACEITO SIM, MEU DONO!*\n%s & %s oficializaram o casamento! 💍", mentionTag(senderPN), mentionTag(botID))
					if err := sendCard(ctx, img, err, cap, senderPN.String(), botJIDStr); err != nil {
						return err
					}
					if mp3, err := os.ReadFile(filepath.Join(ctx.Config.RootDir, "assets", "casamento.mp3")); err == nil && len(mp3) > 0 {
						_ = ctx.SendAudio(mp3, false)
					}
					return nil
				}
			}
			_ = ctx.DB.SetProposal(senderPN.String(), targetPN.String())
			if targetPN.User != target.User {
				_ = ctx.DB.SetProposal(senderPN.String(), target.ToNonAD().String())
			}
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("💍 %s pediu %s em casamento!\nUse *%saceitar* para aceitar.", mentionTag(senderPN), mentionTag(targetPN), ctx.Prefix), senderPN.String(), targetPN.String())
		},
	})
	r.Register(&Command{
		Name: "aceitar", Category: "resenha", Description: "Aceita pedido de casamento",
		Execute: func(ctx *Context) error {
			senderPN := ctx.Bot.ResolvePN(ctx.Sender).ToNonAD()
			candidates := []string{senderPN.String(), ctx.Sender.ToNonAD().String()}
			if ctx.Event != nil {
				candidates = append(candidates, ctx.Event.Info.Sender.ToNonAD().String())
			}
			var from, matchedKey string
			for _, k := range candidates {
				if f := ctx.DB.GetProposal(k); f != "" {
					from = f
					matchedKey = k
					break
				}
			}
			if from == "" {
				return ctx.Reply("❌ Você não tem pedidos pendentes.")
			}
			for _, k := range candidates {
				_ = ctx.DB.ClearProposal(k)
			}
			_ = matchedKey
			now := time.Now().UnixMilli()
			u1 := ctx.DB.GetUser(from)
			u2 := ctx.DB.GetUser(senderPN.String())
			u1.Spouse = senderPN.String()
			u1.MarriedAt = now
			u2.Spouse = from
			u2.MarriedAt = now
			_ = ctx.DB.SaveUser(u1)
			_ = ctx.DB.SaveUser(u2)
			fromJID, _ := types.ParseJID(from)
			img, err := renderCasamentoCard(ctx.Config.RootDir, "@"+fromJID.User, "@"+senderPN.User, 100, fetchAvatar(ctx, fromJID), fetchAvatar(ctx, senderPN))
			cap := fmt.Sprintf("💖 *FELICIDADES AOS NOIVOS!*\n%s & %s oficializaram o casamento! 💍", mentionTag(fromJID), mentionTag(senderPN))
			if err := sendCard(ctx, img, err, cap, from, senderPN.String()); err != nil {
				return err
			}
			if mp3, err := os.ReadFile(filepath.Join(ctx.Config.RootDir, "assets", "casamento.mp3")); err == nil && len(mp3) > 0 {
				_ = ctx.SendAudio(mp3, false)
			}
			return nil
		},
	})
	r.Register(&Command{
		Name: "divorcio", Aliases: []string{"divórcio", "separar"}, Category: "resenha", Description: "Encerra o casamento atual",
		Execute: func(ctx *Context) error {
			u := ctx.DB.GetUser(ctx.Sender.String())
			if u.Spouse == "" {
				return ctx.Reply("❌ Você não está casado(a).")
			}
			ex := ctx.DB.GetUser(u.Spouse)
			ex.Spouse = ""
			ex.MarriedAt = 0
			u.Spouse = ""
			u.MarriedAt = 0
			_ = ctx.DB.SaveUser(ex)
			_ = ctx.DB.SaveUser(u)
			return ctx.Reply("💔 Divórcio concluído.")
		},
	})

	// Adulto (18+)
	r.Register(&Command{
		Name: "nsfw", Aliases: []string{"pack", "gostosa"}, Category: "adulto", Premium: true,
		Description: "Conteúdo +18 (VIP)",
		Execute: func(ctx *Context) error {
			var res struct {
				URL string `json:"url"`
			}
			if err := httpGetJSON("https://api.waifu.pics/nsfw/waifu", &res); err != nil || res.URL == "" {
				return ctx.Reply("❌ Indisponível.")
			}
			b, err := httpGetBytes(res.URL)
			if err != nil {
				return ctx.Reply("❌ Erro ao baixar.")
			}
			return ctx.SendImage(b, "🔞")
		},
	})

	// Memória da Lunar
	r.Register(&Command{
		Name: "lembrancas", Aliases: []string{"lembrar", "memories"}, Category: "extras",
		Description: "Mostra o que a Lunar lembra sobre você",
		Execute: func(ctx *Context) error {
			facts := ctx.DB.GetLunarMemories("user:"+ctx.Sender.ToNonAD().String(), 20)
			if len(facts) == 0 {
				return ctx.Reply("🧠 Ainda não guardei nenhum fato permanente sobre você.")
			}
			var sb strings.Builder
			sb.WriteString("🧠 *O que eu lembro sobre você:*\n\n")
			for i, f := range facts {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
			}
			return ctx.Reply(sb.String())
		},
	})

	r.Register(&Command{
		Name: "esquecer", Aliases: []string{"limparchat"}, Category: "extras",
		Description: "Apaga lembranças da Lunar (.esquecer [termo|tudo|chat])",
		Execute: func(ctx *Context) error {
			q := strings.TrimSpace(ctx.Q)
			if strings.EqualFold(q, "chat") {
				if err := ctx.DB.ClearLunarHistory(ctx.From.String()); err != nil {
					return ctx.Reply("❌ Erro ao limpar histórico do chat.")
				}
				return ctx.Reply("🧹 Histórico de conversa deste chat limpo.")
			}
			senderKey := "user:" + ctx.Sender.ToNonAD().String()
			if strings.EqualFold(q, "tudo") || q == "" {
				n, _ := ctx.DB.DeleteLunarMemory(senderKey, "")
				return ctx.Reply(fmt.Sprintf("🧹 Apaguei %d lembrança(s) suas.", n))
			}
			n, _ := ctx.DB.DeleteLunarMemory(senderKey, q)
			return ctx.Reply(fmt.Sprintf("🧹 Apaguei %d lembrança(s) com %q.", n, q))
		},
	})
}

package commands

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nyxbot-go/internal/db"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func randInt(min, max int) int {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

func registerUtilidades(r *Registry) {
	r.Register(&Command{
		Name: "ping", Category: "utilidades", Description: "Mede latência do bot",
		Execute: func(ctx *Context) error {
			start := time.Now()
			ctx.React("🏓")
			lat := time.Since(start).Milliseconds()
			return ctx.Reply(fmt.Sprintf("🏓 *Pong!*\n⚡ Latência: *%dms*\n⏱️ Uptime: *%s*", lat, time.Since(startTime).Round(time.Second)))
		},
	})

	r.Register(&Command{
		Name: "pinghost", Category: "utilidades", Description: "Testa conexão TCP em um host",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o host (ex: google.com:443)")
			}
			host := ctx.Q
			if !strings.Contains(host, ":") {
				host += ":80"
			}
			t0 := time.Now()
			conn, err := net.DialTimeout("tcp", host, 5*time.Second)
			if err != nil {
				return ctx.Reply("❌ Falha: " + err.Error())
			}
			conn.Close()
			return ctx.Reply(fmt.Sprintf("🌐 *%s* respondeu em *%dms*", host, time.Since(t0).Milliseconds()))
		},
	})

	r.Register(&Command{
		Name: "menu", Aliases: []string{"ajuda", "help", "commands", "comandos"}, Category: "utilidades",
		Description: "Exibe o menu de comandos do bot",
		Execute: func(ctx *Context) error {
			ctx.React("🌸")
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🩸 *%s* (Go Edition)\n", ctx.Config.NomeDoBot))
			sb.WriteString(fmt.Sprintf("👤 Olá, *%s*!\n", ctx.PushName))
			sb.WriteString(fmt.Sprintf("🔑 Prefixo: *%s* | Comandos: *%d*\n", ctx.Prefix, len(ctx.Registry.AllUnique())))
			sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")

			cats := ctx.Registry.Categories()
			filter := strings.ToLower(ctx.Q)
			for _, cat := range cats {
				if filter != "" && !strings.Contains(strings.ToLower(cat), filter) {
					continue
				}
				cmds := ctx.Registry.ListByCategory(cat)
				sb.WriteString(fmt.Sprintf("\n📂 *%s* (%d)\n", strings.ToUpper(cat), len(cmds)))
				var names []string
				for _, c := range cmds {
					names = append(names, ctx.Prefix+c.Name)
				}
				sb.WriteString(strings.Join(names, " • ") + "\n")
			}
			sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━\n")
			sb.WriteString(fmt.Sprintf("💡 Use *%smenu <categoria>* para filtrar.", ctx.Prefix))

			mp4Path := filepath.Join(ctx.Config.RootDir, "assets", "menu-bg.mp4")
			if data, err := os.ReadFile(mp4Path); err == nil {
				return ctx.SendVideo(data, sb.String(), true)
			}
			bgPath := filepath.Join(ctx.Config.RootDir, "assets", "menu-bg.jpeg")
			if data, err := os.ReadFile(bgPath); err == nil {
				return ctx.SendImage(data, sb.String())
			}
			return ctx.Reply(sb.String())
		},
	})

	r.Register(&Command{
		Name: "meuid", Category: "utilidades", Description: "Mostra seu JID e ID do chat",
		Execute: func(ctx *Context) error {
			return ctx.Reply(fmt.Sprintf("🆔 *Seu ID:* `%s`\n💬 *Chat:* `%s`", ctx.Sender.String(), ctx.From.String()))
		},
	})

	r.Register(&Command{
		Name: "quemdono", Aliases: []string{"dono"}, Category: "utilidades", Description: "Mostra informações do dono",
		Execute: func(ctx *Context) error {
			return ctx.Reply(fmt.Sprintf("👑 *Dono:* %s\n📱 *Contato:* wa.me/%s", ctx.Config.NomeDoDono, configClean(ctx.Config.NumeroDoDono)))
		},
	})

	r.Register(&Command{
		Name: "aluguel", Category: "utilidades", OnlyGroup: true, Description: "Verifica status de ativação do grupo",
		Execute: func(ctx *Context) error {
			exists, exp := ctx.DB.GetGroupExpiry(ctx.From.String())
			if !exists {
				return ctx.Reply("✅ Grupo sem restrição de aluguel.")
			}
			if exp == 0 {
				return ctx.Reply("✅ Grupo ativo *permanentemente*.")
			}
			if exp < time.Now().UnixMilli() {
				return ctx.Reply("⛔ Aluguel expirado. Peça ao dono para renovar.")
			}
			rem := time.Until(time.UnixMilli(exp)).Round(time.Hour)
			return ctx.Reply(fmt.Sprintf("✅ Grupo ativo! Expira em *%s* (%s).", rem, time.UnixMilli(exp).Format("02/01/2006")))
		},
	})

	r.Register(&Command{
		Name: "perfil", Aliases: []string{"profile"}, Category: "utilidades", Description: "Mostra seu perfil de economia e nível",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				target = ctx.Sender
			}
			u := ctx.DB.GetUser(target.String())
			need := db.XPForLevel(u.Level, ctx.Config.Niveis.XpBase, ctx.Config.Niveis.Multiplicador)
			spouseTxt := "Solteiro(a)"
			if u.Spouse != "" {
				spouseTxt = "@" + strings.Split(u.Spouse, "@")[0]
			}
			return ctx.Reply(fmt.Sprintf(
				"👤 *PERFIL*\n\n"+
					"🪙 %s: *%d* %s\n"+
					"⭐ Nível: *%d* (%d/%d XP)\n"+
					"🏆 Vitórias: *%d* | 💀 Derrotas: *%d*\n"+
					"💍 Estado civil: %s",
				ctx.Config.Moeda.Nome, u.Coins, ctx.Config.Moeda.Simbolo,
				u.Level, u.XP, need, u.Wins, u.Losses, spouseTxt,
			))
		},
	})

	r.Register(&Command{
		Name: "saldo", Aliases: []string{"coins", "carteira"}, Category: "utilidades", Description: "Consulta seu saldo",
		Execute: func(ctx *Context) error {
			u := ctx.DB.GetUser(ctx.Sender.String())
			return ctx.Reply(fmt.Sprintf("%s Seu saldo: *%d %s*", ctx.Config.Moeda.Simbolo, u.Coins, ctx.Config.Moeda.Nome))
		},
	})

	r.Register(&Command{
		Name: "daily", Category: "utilidades", Description: "Resgata recompensa diária",
		Execute: func(ctx *Context) error {
			u := ctx.DB.GetUser(ctx.Sender.String())
			now := time.Now().UnixMilli()
			cd := int64(24 * time.Hour / time.Millisecond)
			if now-u.Daily < cd {
				rem := time.Duration(cd-(now-u.Daily)) * time.Millisecond
				return ctx.Reply(fmt.Sprintf("⏳ Você já resgatou hoje! Volte em *%s*.", rem.Round(time.Minute)))
			}
			reward := int64(randInt(int(ctx.Config.Moeda.DailyMin), int(ctx.Config.Moeda.DailyMax)))
			u.Coins += reward
			u.Daily = now
			_ = ctx.DB.SaveUser(u)
			return ctx.Reply(fmt.Sprintf("🎁 Você resgatou *%d %s* %s!\n💰 Novo saldo: *%d*", reward, ctx.Config.Moeda.Nome, ctx.Config.Moeda.Simbolo, u.Coins))
		},
	})

	r.Register(&Command{
		Name: "level", Aliases: []string{"xp"}, Category: "utilidades", Description: "Mostra seu nível atual",
		Execute: func(ctx *Context) error {
			u := ctx.DB.GetUser(ctx.Sender.String())
			need := db.XPForLevel(u.Level, ctx.Config.Niveis.XpBase, ctx.Config.Niveis.Multiplicador)
			return ctx.Reply(fmt.Sprintf("⭐ Nível *%d* — XP: *%d / %d*", u.Level, u.XP, need))
		},
	})

	r.Register(&Command{
		Name: "rank", Aliases: []string{"top", "leaderboard"}, Category: "utilidades", Description: "Ranking de níveis e moedas",
		Execute: func(ctx *Context) error {
			by := "level"
			if strings.EqualFold(ctx.Q, "coins") || strings.EqualFold(ctx.Q, "saldo") {
				by = "coins"
			}
			users := ctx.DB.TopUsers(by, 10)
			if len(users) == 0 {
				return ctx.Reply("📭 Ranking vazio.")
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🏆 *RANKING (%s)*\n\n", strings.ToUpper(by)))
			for i, u := range users {
				tag := strings.Split(u.JID, "@")[0]
				sb.WriteString(fmt.Sprintf("%d. @%s — Nv.*%d* | %d %s\n", i+1, tag, u.Level, u.Coins, ctx.Config.Moeda.Simbolo))
			}
			return ctx.Reply(sb.String())
		},
	})

	r.Register(&Command{
		Name: "loja", Category: "utilidades", Description: "Loja de itens do bot",
		Execute: func(ctx *Context) error {
			items := map[string]struct {
				name  string
				price int64
			}{
				"1": {"💍 Aliança", 500},
				"2": {"🛡️ Escudo", 300},
				"3": {"🍀 Trevo da Sorte", 250},
				"4": {"💎 Passe VIP (1d)", 2000},
			}
			if ctx.Q == "" {
				return ctx.Reply("🛒 *LOJA LUNAR*\n\n1. 💍 Aliança — 500\n2. 🛡️ Escudo — 300\n3. 🍀 Trevo da Sorte — 250\n4. 💎 Passe VIP (1d) — 2000\n\nUse: *.loja <número>*")
			}
			it, ok := items[ctx.Args[0]]
			if !ok {
				return ctx.Reply("❗ Item inválido.")
			}
			u := ctx.DB.GetUser(ctx.Sender.String())
			if u.Coins < it.price {
				return ctx.Reply(fmt.Sprintf("❌ Saldo insuficiente! Precisa de *%d*.", it.price))
			}
			u.Coins -= it.price
			u.Inventory = append(u.Inventory, it.name)
			_ = ctx.DB.SaveUser(u)
			if ctx.Args[0] == "4" {
				_ = ctx.DB.AddPremium(ctx.Sender.String(), 1)
			}
			return ctx.Reply(fmt.Sprintf("✅ Você comprou *%s* por *%d*!", it.name, it.price))
		},
	})

	r.Register(&Command{
		Name: "afk", Category: "utilidades", Description: "Define status AFK (ausente)",
		Execute: func(ctx *Context) error {
			reason := ctx.Q
			if reason == "" {
				reason = "Ausente no momento"
			}
			_ = ctx.DB.SetAFK(ctx.Sender.String(), reason)
			return ctx.Reply(fmt.Sprintf("💤 Modo AFK ativado: *%s*", reason))
		},
	})

	r.Register(&Command{
		Name: "autoreply", Category: "utilidades", Admin: true, OnlyGroup: true,
		Description: "Gerencia respostas automáticas (.autoreply add gatilho|resposta / del gatilho / list)",
		Execute: func(ctx *Context) error {
			if len(ctx.Args) == 0 || ctx.Args[0] == "list" {
				m := ctx.DB.GetAutoreplies(ctx.From.String())
				if len(m) == 0 {
					return ctx.Reply("📭 Nenhuma auto-resposta configurada.")
				}
				var sb strings.Builder
				sb.WriteString("🤖 *Auto-respostas:*\n")
				for k, v := range m {
					sb.WriteString(fmt.Sprintf("• `%s` → %s\n", k, v))
				}
				return ctx.Reply(sb.String())
			}
			sub := strings.ToLower(ctx.Args[0])
			rest := strings.TrimSpace(strings.TrimPrefix(ctx.Q, ctx.Args[0]))
			if sub == "add" {
				parts := strings.SplitN(rest, "|", 2)
				if len(parts) != 2 {
					return ctx.Reply("❗ Use: *.autoreply add palavra | resposta*")
				}
				_ = ctx.DB.SetAutoreply(ctx.From.String(), strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				return ctx.Reply("✅ Auto-resposta adicionada!")
			}
			if sub == "del" {
				_ = ctx.DB.DelAutoreply(ctx.From.String(), rest)
				return ctx.Reply("✅ Auto-resposta removida!")
			}
			return ctx.Reply("❗ Use: add, del ou list.")
		},
	})

	r.Register(&Command{
		Name: "autoreplyfaq", Category: "utilidades", Description: "Ajuda sobre autoreply",
		Execute: func(ctx *Context) error {
			return ctx.Reply("🤖 *Autoreply FAQ*\nAdicione: `.autoreply add oi | Olá!`\nRemova: `.autoreply del oi`\nListe: `.autoreply list`")
		},
	})

	r.Register(&Command{
		Name: "ia", Aliases: []string{"gpt", "gemini"}, Category: "utilidades",
		Description: "Pergunta para a IA (Gemini)",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Escreva sua pergunta.")
			}
			key := ctx.Config.GeminiAPIKey
			if key == "" {
				return ctx.Reply("⚠️ `GEMINI_API_KEY` não configurada.")
			}
			endpoint := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=" + url.QueryEscape(key)
			payload := map[string]any{
				"contents": []map[string]any{
					{"parts": []map[string]string{{"text": ctx.Q}}},
				},
			}
			var res struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}
			if err := httpPostJSON(endpoint, payload, nil, &res); err != nil {
				return ctx.Reply("❌ Erro na IA: " + err.Error())
			}
			if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
				return ctx.Reply("❌ Sem resposta da IA.")
			}
			return ctx.Reply("🤖 " + res.Candidates[0].Content.Parts[0].Text)
		},
	})

	r.Register(&Command{
		Name: "aistatus", Category: "utilidades", Description: "Verifica status da chave Gemini",
		Execute: func(ctx *Context) error {
			if ctx.Config.GeminiAPIKey == "" {
				return ctx.Reply("🔴 IA offline (sem chave configurada).")
			}
			return ctx.Reply("🟢 IA Gemini configurada e pronta.")
		},
	})

	r.Register(&Command{
		Name: "revela", Aliases: []string{".", "salvarviewonce"}, Category: "utilidades",
		Description: "Revela mídia de visualização única respondida",
		Execute: func(ctx *Context) error {
			qm := getQuotedMessage(ctx)
			if qm == nil {
				return ctx.Reply("❗ Responda uma mídia de visualização única.")
			}
			inner := unwrapViewOnce(qm)
			if inner.ImageMessage != nil {
				data, err := ctx.Bot.Raw().Download(context.Background(), inner.ImageMessage)
				if err != nil {
					return ctx.Reply("❌ Erro ao baixar imagem: " + err.Error())
				}
				return ctx.SendImage(data, inner.ImageMessage.GetCaption())
			}
			if inner.VideoMessage != nil {
				data, err := ctx.Bot.Raw().Download(context.Background(), inner.VideoMessage)
				if err != nil {
					return ctx.Reply("❌ Erro ao baixar vídeo: " + err.Error())
				}
				return ctx.SendVideo(data, inner.VideoMessage.GetCaption(), false)
			}
			if inner.AudioMessage != nil {
				data, err := ctx.Bot.Raw().Download(context.Background(), inner.AudioMessage)
				if err != nil {
					return ctx.Reply("❌ Erro ao baixar áudio: " + err.Error())
				}
				return ctx.SendAudio(data, inner.AudioMessage.GetPTT())
			}
			return ctx.Reply("❗ A mensagem respondida não contém mídia.")
		},
	})

	r.Register(&Command{
		Name: "tagall", Category: "utilidades", Admin: true, OnlyGroup: true,
		Description: "Alias para marcar todos",
		Execute: func(ctx *Context) error {
			return r.Get("marcar").Execute(ctx)
		},
	})

	r.Register(&Command{
		Name: "qrcode", Category: "utilidades", Description: "Gera QR Code de um texto",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o texto ou link.")
			}
			png, err := qrcode.Encode(ctx.Q, qrcode.Medium, 512)
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.SendImage(png, "✅ QR Code gerado!")
		},
	})

	r.Register(&Command{
		Name: "cep", Category: "utilidades", Description: "Consulta CEP via ViaCEP",
		Execute: func(ctx *Context) error {
			cep := cleanDigits(ctx.Q)
			if len(cep) != 8 {
				return ctx.Reply("❗ Informe um CEP válido (8 dígitos).")
			}
			var res struct {
				CEP        string `json:"cep"`
				Logradouro string `json:"logradouro"`
				Bairro     string `json:"bairro"`
				Localidade string `json:"localidade"`
				UF         string `json:"uf"`
				Erro       bool   `json:"erro"`
			}
			if err := httpGetJSON("https://viacep.com.br/ws/"+cep+"/json/", &res); err != nil || res.Erro {
				return ctx.Reply("❌ CEP não encontrado.")
			}
			return ctx.Reply(fmt.Sprintf("📮 *CEP %s*\n📍 %s\n🏘️ %s\n🏙️ %s - %s", res.CEP, res.Logradouro, res.Bairro, res.Localidade, res.UF))
		},
	})

	r.Register(&Command{
		Name: "clima", Category: "utilidades", Description: "Consulta clima de uma cidade",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe a cidade.")
			}
			b, err := httpGetBytes("https://wttr.in/" + url.PathEscape(ctx.Q) + "?format=%l:+%c+%t+(%f)+%w+%h&lang=pt")
			if err != nil {
				return ctx.Reply("❌ Erro ao consultar clima.")
			}
			return ctx.Reply("🌤️ " + strings.TrimSpace(string(b)))
		},
	})

	r.Register(&Command{
		Name: "cotacao", Aliases: []string{"moeda_convert"}, Category: "utilidades", Description: "Cotação USD/EUR/BTC em BRL",
		Execute: func(ctx *Context) error {
			var res map[string]struct {
				Name string `json:"name"`
				Bid  string `json:"bid"`
			}
			if err := httpGetJSON("https://economia.awesomeapi.com.br/last/USD-BRL,EUR-BRL,BTC-BRL", &res); err != nil {
				return ctx.Reply("❌ Erro ao consultar cotações.")
			}
			return ctx.Reply(fmt.Sprintf(
				"💱 *COTAÇÕES*\n\n🇺🇸 Dólar: R$ %s\n🇪🇺 Euro: R$ %s\n₿ Bitcoin: R$ %s",
				res["USDBRL"].Bid, res["EURBRL"].Bid, res["BTCBRL"].Bid,
			))
		},
	})

	r.Register(&Command{
		Name: "ddd", Category: "utilidades", Description: "Consulta estado e cidades de um DDD",
		Execute: func(ctx *Context) error {
			ddd := cleanDigits(ctx.Q)
			if len(ddd) != 2 {
				return ctx.Reply("❗ Informe um DDD de 2 dígitos.")
			}
			var res struct {
				State  string   `json:"state"`
				Cities []string `json:"cities"`
			}
			if err := httpGetJSON("https://brasilapi.com.br/api/ddd/v1/"+ddd, &res); err != nil || res.State == "" {
				return ctx.Reply("❌ DDD não encontrado.")
			}
			limit := len(res.Cities)
			if limit > 15 {
				limit = 15
			}
			return ctx.Reply(fmt.Sprintf("📞 *DDD %s* — Estado: *%s*\n🏙️ Cidades: %s...", ddd, res.State, strings.Join(res.Cities[:limit], ", ")))
		},
	})

	r.Register(&Command{
		Name: "ipinfo", Category: "utilidades", Description: "Consulta informações de um IP",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe um IP.")
			}
			var res struct {
				Query   string `json:"query"`
				Country string `json:"country"`
				Region  string `json:"regionName"`
				City    string `json:"city"`
				ISP     string `json:"isp"`
			}
			if err := httpGetJSON("http://ip-api.com/json/"+url.PathEscape(ctx.Q), &res); err != nil {
				return ctx.Reply("❌ Erro ao consultar IP.")
			}
			return ctx.Reply(fmt.Sprintf("🌐 *IP:* %s\n🏳️ %s (%s - %s)\n📡 ISP: %s", res.Query, res.Country, res.City, res.Region, res.ISP))
		},
	})

	r.Register(&Command{
		Name: "github", Category: "utilidades", Description: "Consulta perfil do GitHub",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o usuário do GitHub.")
			}
			var res struct {
				Login       string `json:"login"`
				Name        string `json:"name"`
				Bio         string `json:"bio"`
				PublicRepos int    `json:"public_repos"`
				Followers   int    `json:"followers"`
				HTMLURL     string `json:"html_url"`
			}
			if err := httpGetJSON("https://api.github.com/users/"+url.PathEscape(ctx.Q), &res); err != nil || res.Login == "" {
				return ctx.Reply("❌ Usuário não encontrado.")
			}
			return ctx.Reply(fmt.Sprintf("🐙 *%s* (%s)\n📝 %s\n📦 Repos: *%d* | 👥 Seguidores: *%d*\n🔗 %s", res.Login, res.Name, res.Bio, res.PublicRepos, res.Followers, res.HTMLURL))
		},
	})

	r.Register(&Command{
		Name: "anime", Category: "utilidades", Description: "Busca informações de um anime",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o nome do anime.")
			}
			var res struct {
				Data []struct {
					Title    string  `json:"title"`
					Score    float64 `json:"score"`
					Episodes int     `json:"episodes"`
					Status   string  `json:"status"`
					URL      string  `json:"url"`
				} `json:"data"`
			}
			if err := httpGetJSON("https://api.jikan.moe/v4/anime?limit=1&q="+url.QueryEscape(ctx.Q), &res); err != nil || len(res.Data) == 0 {
				return ctx.Reply("❌ Anime não encontrado.")
			}
			a := res.Data[0]
			return ctx.Reply(fmt.Sprintf("🎌 *%s*\n⭐ Nota: *%.2f*\n📺 Episódios: *%d*\n📌 Status: %s\n🔗 %s", a.Title, a.Score, a.Episodes, a.Status, a.URL))
		},
	})

	r.Register(&Command{
		Name: "serie", Category: "utilidades", Description: "Busca informações de uma série",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o nome da série.")
			}
			var res struct {
				Name    string `json:"name"`
				Status  string `json:"status"`
				Summary string `json:"summary"`
				URL     string `json:"url"`
			}
			if err := httpGetJSON("https://api.tvmaze.com/singlesearch/shows?q="+url.QueryEscape(ctx.Q), &res); err != nil || res.Name == "" {
				return ctx.Reply("❌ Série não encontrada.")
			}
			return ctx.Reply(fmt.Sprintf("📺 *%s* (%s)\n🔗 %s", res.Name, res.Status, res.URL))
		},
	})

	r.Register(&Command{
		Name: "traduzir", Category: "utilidades", Description: "Traduz texto para português (.traduzir <texto>)",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o texto para traduzir.")
			}
			var res struct {
				ResponseData struct {
					TranslatedText string `json:"translatedText"`
				} `json:"responseData"`
			}
			u := "https://api.mymemory.translated.net/get?q=" + url.QueryEscape(ctx.Q) + "&langpair=en|pt"
			if err := httpGetJSON(u, &res); err != nil {
				return ctx.Reply("❌ Erro na tradução.")
			}
			return ctx.Reply("🌐 *Tradução:*\n" + res.ResponseData.TranslatedText)
		},
	})

	r.Register(&Command{
		Name: "encurtar", Category: "utilidades", Description: "Encurta uma URL",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe a URL.")
			}
			b, err := httpGetBytes("https://is.gd/create.php?format=simple&url=" + url.QueryEscape(ctx.Q))
			if err != nil {
				return ctx.Reply("❌ Erro ao encurtar.")
			}
			return ctx.Reply("🔗 " + strings.TrimSpace(string(b)))
		},
	})

	r.Register(&Command{
		Name: "enquete", Category: "utilidades", Description: "Cria uma enquete (.enquete Pergunta | Op1 | Op2)",
		Execute: func(ctx *Context) error {
			parts := strings.Split(ctx.Q, "|")
			if len(parts) < 3 {
				return ctx.Reply("❗ Use: *.enquete Pergunta | Opção 1 | Opção 2*")
			}
			q := strings.TrimSpace(parts[0])
			var opts []string
			for _, p := range parts[1:] {
				if t := strings.TrimSpace(p); t != "" {
					opts = append(opts, t)
				}
			}
			msg := ctx.Bot.Raw().BuildPollCreation(q, opts, 1)
			_, err := ctx.Bot.Raw().SendMessage(context.Background(), ctx.From, msg)
			return err
		},
	})

	r.Register(&Command{
		Name: "sorteio", Category: "utilidades", Description: "Sorteia um item ou participante",
		Execute: func(ctx *Context) error {
			if ctx.Q != "" {
				items := strings.Split(ctx.Q, ",")
				if len(items) == 1 {
					items = ctx.Args
				}
				pick := strings.TrimSpace(items[randInt(0, len(items)-1)])
				return ctx.Reply("🎲 Sorteado: *" + pick + "*")
			}
			if ctx.IsGroup && ctx.GroupInfo != nil && len(ctx.GroupInfo.Participants) > 0 {
				p := ctx.GroupInfo.Participants[randInt(0, len(ctx.GroupInfo.Participants)-1)]
				return ctx.Bot.SendText(ctx.From, fmt.Sprintf("🎉 Sorteado: %s!", mentionTag(p.JID)), p.JID.String())
			}
			return ctx.Reply("❗ Informe opções separadas por vírgula.")
		},
	})

	r.Register(&Command{
		Name: "lembrete", Category: "utilidades", Description: "Define um lembrete (.lembrete 5m Tomar água)",
		Execute: func(ctx *Context) error {
			if len(ctx.Args) < 2 {
				return ctx.Reply("❗ Use: *.lembrete 5m Mensagem*")
			}
			d, err := time.ParseDuration(ctx.Args[0])
			if err != nil || d <= 0 || d > 24*time.Hour {
				return ctx.Reply("❗ Duração inválida (use ex: 30s, 5m, 1h).")
			}
			msg := strings.TrimSpace(strings.TrimPrefix(ctx.Q, ctx.Args[0]))
			go func() {
				time.Sleep(d)
				_ = ctx.Bot.SendText(ctx.From, fmt.Sprintf("⏰ *Lembrete para %s:*\n%s", mentionTag(ctx.Sender), msg), ctx.Sender.String())
			}()
			return ctx.Reply(fmt.Sprintf("⏰ Lembrete definido para daqui *%s*!", d))
		},
	})

	r.Register(&Command{
		Name: "calcular", Aliases: []string{"calc"}, Category: "utilidades", Description: "Calculadora básica (+ - * /)",
		Execute: func(ctx *Context) error {
			if len(ctx.Args) < 3 {
				return ctx.Reply("❗ Use: *.calcular 10 + 5*")
			}
			a, err1 := strconv.ParseFloat(ctx.Args[0], 64)
			op := ctx.Args[1]
			b, err2 := strconv.ParseFloat(ctx.Args[2], 64)
			if err1 != nil || err2 != nil {
				return ctx.Reply("❗ Números inválidos.")
			}
			var res float64
			switch op {
			case "+":
				res = a + b
			case "-":
				res = a - b
			case "*", "x":
				res = a * b
			case "/":
				if b == 0 {
					return ctx.Reply("❌ Divisão por zero.")
				}
				res = a / b
			default:
				return ctx.Reply("❗ Operador inválido (+, -, *, /).")
			}
			return ctx.Reply(fmt.Sprintf("🧮 Resultado: *%g*", res))
		},
	})

	// Text encoders/utilities
	r.Register(&Command{Name: "base64", Category: "utilidades", Description: "Codifica em Base64", Execute: func(ctx *Context) error {
		return ctx.Reply(base64.StdEncoding.EncodeToString([]byte(ctx.Q)))
	}})
	r.Register(&Command{Name: "deb64", Category: "utilidades", Description: "Decodifica Base64", Execute: func(ctx *Context) error {
		b, err := base64.StdEncoding.DecodeString(ctx.Q)
		if err != nil {
			return ctx.Reply("❌ Base64 inválido.")
		}
		return ctx.Reply(string(b))
	}})
	r.Register(&Command{Name: "hex", Category: "utilidades", Description: "Converte para Hexadecimal", Execute: func(ctx *Context) error {
		return ctx.Reply(hex.EncodeToString([]byte(ctx.Q)))
	}})
	r.Register(&Command{Name: "binario", Category: "utilidades", Description: "Converte texto para binário", Execute: func(ctx *Context) error {
		var parts []string
		for _, b := range []byte(ctx.Q) {
			parts = append(parts, fmt.Sprintf("%08b", b))
		}
		return ctx.Reply(strings.Join(parts, " "))
	}})
	r.Register(&Command{Name: "frombinario", Category: "utilidades", Description: "Converte binário para texto", Execute: func(ctx *Context) error {
		var out []byte
		for _, tok := range strings.Fields(ctx.Q) {
			v, err := strconv.ParseUint(tok, 2, 8)
			if err == nil {
				out = append(out, byte(v))
			}
		}
		return ctx.Reply(string(out))
	}})
	r.Register(&Command{Name: "morse", Category: "utilidades", Description: "Converte texto para código Morse", Execute: func(ctx *Context) error {
		m := map[rune]string{'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".", 'F': "..-.", 'G': "--.", 'H': "....", 'I': "..", 'J': ".---", 'K': "-.-", 'L': ".-..", 'M': "--", 'N': "-.", 'O': "---", 'P': ".--.", 'Q': "--.-", 'R': ".-.", 'S': "...", 'T': "-", 'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-", 'Y': "-.--", 'Z': "--..", ' ': "/"}
		var out []string
		for _, r := range strings.ToUpper(ctx.Q) {
			if code, ok := m[r]; ok {
				out = append(out, code)
			}
		}
		return ctx.Reply(strings.Join(out, " "))
	}})
	r.Register(&Command{Name: "ascii", Category: "utilidades", Description: "Códigos ASCII do texto", Execute: func(ctx *Context) error {
		var out []string
		for _, b := range []byte(ctx.Q) {
			out = append(out, strconv.Itoa(int(b)))
		}
		return ctx.Reply(strings.Join(out, " "))
	}})
	r.Register(&Command{Name: "maiuscula", Category: "utilidades", Description: "Texto em MAIÚSCULAS", Execute: func(ctx *Context) error {
		return ctx.Reply(strings.ToUpper(ctx.Q))
	}})
	r.Register(&Command{Name: "minuscula", Category: "utilidades", Description: "Texto em minúsculas", Execute: func(ctx *Context) error {
		return ctx.Reply(strings.ToLower(ctx.Q))
	}})
	r.Register(&Command{Name: "invertertexto", Category: "utilidades", Description: "Inverte um texto", Execute: func(ctx *Context) error {
		runes := []rune(ctx.Q)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return ctx.Reply(string(runes))
	}})
	r.Register(&Command{Name: "contar", Category: "utilidades", Description: "Conta caracteres e palavras", Execute: func(ctx *Context) error {
		return ctx.Reply(fmt.Sprintf("📝 Caracteres: *%d* | Palavras: *%d*", len([]rune(ctx.Q)), len(strings.Fields(ctx.Q))))
	}})
	r.Register(&Command{Name: "palindromo", Category: "utilidades", Description: "Verifica se é palíndromo", Execute: func(ctx *Context) error {
		clean := strings.ToLower(strings.ReplaceAll(ctx.Q, " ", ""))
		runes := []rune(clean)
		ok := true
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			if runes[i] != runes[j] {
				ok = false
				break
			}
		}
		if ok {
			return ctx.Reply("✅ É um palíndromo!")
		}
		return ctx.Reply("❌ Não é um palíndromo.")
	}})
	r.Register(&Command{Name: "gerarsenha", Aliases: []string{"senha"}, Category: "utilidades", Description: "Gera senha aleatória", Execute: func(ctx *Context) error {
		n := 16
		if len(ctx.Args) > 0 {
			if v, err := strconv.Atoi(ctx.Args[0]); err == nil && v >= 4 && v <= 64 {
				n = v
			}
		}
		const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%&*"
		b := make([]byte, n)
		for i := range b {
			b[i] = chars[randInt(0, len(chars)-1)]
		}
		return ctx.Reply("🔐 `" + string(b) + "`")
	}})
	r.Register(&Command{Name: "uuid", Category: "utilidades", Description: "Gera UUID v4", Execute: func(ctx *Context) error {
		return ctx.Reply("🆔 `" + uuid.NewString() + "`")
	}})
	r.Register(&Command{Name: "gerarnome", Category: "utilidades", Description: "Gera nome aleatório", Execute: func(ctx *Context) error {
		nomes := []string{"Lucas", "Gabriel", "Rafael", "Mariana", "Beatriz", "Camila", "Thiago", "Pedro", "Julia", "Larissa"}
		sobs := []string{"Silva", "Santos", "Oliveira", "Souza", "Pereira", "Costa", "Rodrigues", "Almeida"}
		return ctx.Reply("👤 *" + nomes[randInt(0, len(nomes)-1)] + " " + sobs[randInt(0, len(sobs)-1)] + "*")
	}})
	r.Register(&Command{Name: "useragent", Category: "utilidades", Description: "User-Agent aleatório", Execute: func(ctx *Context) error {
		return ctx.Reply("🌐 `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`")
	}})
	r.Register(&Command{Name: "lorem", Category: "utilidades", Description: "Gera texto Lorem Ipsum", Execute: func(ctx *Context) error {
		return ctx.Reply("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.")
	}})
	r.Register(&Command{Name: "emoji", Category: "utilidades", Description: "Emoji aleatório", Execute: func(ctx *Context) error {
		em := []string{"🔥", "💀", "🩸", "⚡", "👑", "🎰", "🚀", "💎", "🐍", "🐺"}
		return ctx.Reply(em[randInt(0, len(em)-1)])
	}})
	r.Register(&Command{Name: "chance", Category: "utilidades", Description: "Calcula chance de algo acontecer", Execute: func(ctx *Context) error {
		return ctx.Reply(fmt.Sprintf("🎲 Chance de *%s*: *%d%%*", ctx.Q, randInt(0, 100)))
	}})
	r.Register(&Command{Name: "conselho", Category: "utilidades", Description: "Conselho aleatório", Execute: func(ctx *Context) error {
		c := []string{"Não tome decisões permanentes baseadas em emoções temporárias.", "Faça o básico bem feito todos os dias.", "Durma antes de responder mensagens difíceis."}
		return ctx.Reply("💡 " + c[randInt(0, len(c)-1)])
	}})
	r.Register(&Command{Name: "fato", Category: "utilidades", Description: "Fato curioso", Execute: func(ctx *Context) error {
		f := []string{"Polvos têm três corações.", "O mel nunca estraga.", "Vênus gira no sentido contrário à maioria dos planetas."}
		return ctx.Reply("🧠 " + f[randInt(0, len(f)-1)])
	}})
	r.Register(&Command{Name: "frase", Category: "utilidades", Description: "Frase motivacional", Execute: func(ctx *Context) error {
		f := []string{"Disciplina vence motivação.", "Feito é melhor que perfeito.", "Consistência constrói impérios."}
		return ctx.Reply("✨ " + f[randInt(0, len(f)-1)])
	}})
	r.Register(&Command{Name: "piada", Category: "utilidades", Description: "Conta uma piada", Execute: func(ctx *Context) error {
		p := []string{"Por que o programador morreu no chuveiro? Porque o shampoo dizia: aplique, enxágue, repita.", "O que o zero disse para o oito? Belo cinto!"}
		return ctx.Reply("😂 " + p[randInt(0, len(p)-1)])
	}})
	r.Register(&Command{Name: "data", Aliases: []string{"horario"}, Category: "utilidades", Description: "Data e hora atual", Execute: func(ctx *Context) error {
		return ctx.Reply("🕒 " + time.Now().Format("02/01/2006 15:04:05"))
	}})
	r.Register(&Command{Name: "idade", Category: "utilidades", Description: "Calcula idade pelo ano de nascimento", Execute: func(ctx *Context) error {
		y, err := strconv.Atoi(ctx.Q)
		if err != nil || y < 1900 || y > time.Now().Year() {
			return ctx.Reply("❗ Informe o ano (ex: .idade 2000)")
		}
		return ctx.Reply(fmt.Sprintf("🎂 Idade aproximada: *%d anos*", time.Now().Year()-y))
	}})
}

func configClean(s string) string {
	return cleanDigits(s)
}

func unwrapViewOnce(m *waE2E.Message) *waE2E.Message {
	if m == nil {
		return &waE2E.Message{}
	}
	if m.ViewOnceMessage != nil && m.ViewOnceMessage.Message != nil {
		return m.ViewOnceMessage.Message
	}
	if m.ViewOnceMessageV2 != nil && m.ViewOnceMessageV2.Message != nil {
		return m.ViewOnceMessageV2.Message
	}
	if m.ViewOnceMessageV2Extension != nil && m.ViewOnceMessageV2Extension.Message != nil {
		return m.ViewOnceMessageV2Extension.Message
	}
	if m.EphemeralMessage != nil && m.EphemeralMessage.Message != nil {
		return unwrapViewOnce(m.EphemeralMessage.Message)
	}
	return proto.Clone(m).(*waE2E.Message)
}

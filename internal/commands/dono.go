package commands

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nyxbot-go/internal/logger"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var startTime = time.Now()

func registerDono(r *Registry) {
	r.Register(&Command{
		Name: "adddono", Category: "dono", Dono: true,
		Description: "Adiciona um dono extra",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou informe o número.")
			}
			_ = ctx.DB.AddDono(target.String())
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("👑 %s adicionado como dono!", mentionTag(target)), target.String())
		},
	})

	r.Register(&Command{
		Name: "deldono", Category: "dono", Dono: true,
		Description: "Remove um dono extra",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou informe o número.")
			}
			_ = ctx.DB.DelDono(target.String())
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("✅ %s removido dos donos.", mentionTag(target)), target.String())
		},
	})

	r.Register(&Command{
		Name: "listdono", Category: "dono", Dono: true,
		Description: "Lista donos extras",
		Execute: func(ctx *Context) error {
			list := ctx.DB.ListDonos()
			return ctx.Reply(fmt.Sprintf("👑 *Dono principal:* %s\n*Extras:*\n%s", ctx.Config.NumeroDoDono, strings.Join(list, "\n")))
		},
	})

	r.Register(&Command{
		Name: "addvip", Category: "dono", Dono: true,
		Description: "Adiciona usuário VIP/Premium (.addvip @user [dias])",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou informe o número.")
			}
			days := 30
			for _, a := range ctx.Args {
				if v, err := strconv.Atoi(a); err == nil && v > 0 && v < 3650 {
					days = v
				}
			}
			_ = ctx.DB.AddPremium(target.String(), days)
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("💎 %s agora é *VIP* por %d dias!", mentionTag(target), days), target.String())
		},
	})

	r.Register(&Command{
		Name: "delvip", Category: "dono", Dono: true,
		Description: "Remove usuário VIP",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou informe o número.")
			}
			_ = ctx.DB.DelPremium(target.String())
			return ctx.Reply("✅ VIP removido.")
		},
	})

	r.Register(&Command{
		Name: "listvip", Category: "dono", Dono: true,
		Description: "Lista usuários VIP",
		Execute: func(ctx *Context) error {
			list := ctx.DB.ListPremium()
			if len(list) == 0 {
				return ctx.Reply("💎 Nenhum VIP ativo.")
			}
			return ctx.Reply("💎 *VIPs Ativos:*\n" + strings.Join(list, "\n"))
		},
	})

	r.Register(&Command{
		Name: "bcgrupo", Aliases: []string{"broadcast"}, Category: "dono", Dono: true,
		Description: "Envia uma mensagem para todos os grupos do bot",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe a mensagem do broadcast.")
			}
			groups, err := ctx.Bot.Raw().GetJoinedGroups(context.Background())
			if err != nil {
				return ctx.Reply("❌ Erro ao listar grupos: " + err.Error())
			}
			sent := 0
			msg := fmt.Sprintf("📢 *TRANSMISSÃO %s*\n\n%s", strings.ToUpper(ctx.Config.NomeDoBot), ctx.Q)
			for _, g := range groups {
				if err := ctx.Bot.SendText(g.JID, msg); err == nil {
					sent++
				}
				time.Sleep(350 * time.Millisecond)
			}
			return ctx.Reply(fmt.Sprintf("✅ Transmissão enviada para *%d/%d* grupos.", sent, len(groups)))
		},
	})

	r.Register(&Command{
		Name: "block", Category: "dono", Dono: true,
		Description: "Bloqueia um usuário",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Informe o usuário.")
			}
			_, err := ctx.Bot.Raw().UpdateBlocklist(context.Background(), target, events.BlocklistChangeActionBlock, "")
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.Reply("🚫 Usuário bloqueado.")
		},
	})

	r.Register(&Command{
		Name: "unblock", Category: "dono", Dono: true,
		Description: "Desbloqueia um usuário",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Informe o usuário.")
			}
			_, err := ctx.Bot.Raw().UpdateBlocklist(context.Background(), target, events.BlocklistChangeActionUnblock, "")
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.Reply("✅ Usuário desbloqueado.")
		},
	})

	r.Register(&Command{
		Name: "join", Aliases: []string{"entrar"}, Category: "dono", Dono: true,
		Description: "Entra em um grupo via link de convite",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o link de convite.")
			}
			code := strings.TrimSpace(ctx.Q)
			code = strings.TrimPrefix(code, "https://chat.whatsapp.com/")
			code = strings.TrimPrefix(code, "http://chat.whatsapp.com/")
			if idx := strings.Index(code, "?"); idx != -1 {
				code = code[:idx]
			}
			jid, err := ctx.Bot.Raw().JoinGroupWithLink(context.Background(), code)
			if err != nil {
				return ctx.Reply("❌ Erro ao entrar: " + err.Error())
			}
			return ctx.Reply("✅ Entrei no grupo: `" + jid.String() + "`")
		},
	})

	r.Register(&Command{
		Name: "sair", Category: "dono", Dono: true, OnlyGroup: true,
		Description: "Faz o bot sair do grupo atual",
		Execute: func(ctx *Context) error {
			_ = ctx.Reply("👋 Saindo do grupo...")
			return ctx.Bot.Raw().LeaveGroup(context.Background(), ctx.From)
		},
	})

	r.Register(&Command{
		Name: "listgrupos", Category: "dono", Dono: true,
		Description: "Lista todos os grupos que o bot participa",
		Execute: func(ctx *Context) error {
			groups, err := ctx.Bot.Raw().GetJoinedGroups(context.Background())
			if err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📋 *Grupos (%d):*\n\n", len(groups)))
			for i, g := range groups {
				sb.WriteString(fmt.Sprintf("%d. *%s* (%d membros)\n`%s`\n", i+1, g.Name, len(g.Participants), g.JID.String()))
			}
			return ctx.Reply(sb.String())
		},
	})

	r.Register(&Command{
		Name: "logs", Category: "dono", Dono: true,
		Description: "Mostra os últimos logs do bot",
		Execute: func(ctx *Context) error {
			lines := logger.Recent(20)
			if len(lines) == 0 {
				return ctx.Reply("📭 Sem logs recentes.")
			}
			return ctx.Reply("📜 *Últimos Logs:*\n```\n" + strings.Join(lines, "\n") + "\n```")
		},
	})

	r.Register(&Command{
		Name: "setprefix", Category: "dono", Admin: true,
		Description: "Altera o prefixo no grupo atual (ou global se no PV)",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o novo prefixo.")
			}
			p := strings.TrimSpace(ctx.Args[0])
			if ctx.IsGroup {
				_ = ctx.DB.SetGroupPrefix(ctx.From.String(), p)
				return ctx.Reply(fmt.Sprintf("✅ Prefixo deste grupo alterado para *%s*", p))
			}
			ctx.Config.Prefix = p
			return ctx.Reply(fmt.Sprintf("✅ Prefixo global alterado para *%s*", p))
		},
	})

	r.Register(&Command{
		Name: "setbio", Category: "dono", Dono: true,
		Description: "Altera a bio/recado do bot",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe a nova bio.")
			}
			txt := ctx.Q
			if err := ctx.Bot.Raw().SetStatusMessage(context.Background(), types.SetStatusInput{Text: &txt}); err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.Reply("✅ Bio atualizada!")
		},
	})

	r.Register(&Command{
		Name: "setnome", Category: "dono", Dono: true,
		Description: "Altera o nome do bot em memória",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o novo nome.")
			}
			ctx.Config.NomeDoBot = ctx.Q
			return ctx.Reply("✅ Nome do bot atualizado para *" + ctx.Q + "*!")
		},
	})

	r.Register(&Command{
		Name: "status", Aliases: []string{"painel"}, Category: "dono", Dono: true,
		Description: "Mostra status do servidor e runtime Go",
		Execute: func(ctx *Context) error {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			uptime := time.Since(startTime).Round(time.Second)
			groups, _ := ctx.Bot.Raw().GetJoinedGroups(context.Background())
			return ctx.Reply(fmt.Sprintf(
				"⚙️ *PAINEL %s (Go)*\n\n"+
					"⏱️ Uptime: *%s*\n"+
					"🧠 Memória Alloc: *%.2f MB*\n"+
					"⚡ Goroutines: *%d*\n"+
					"👥 Grupos: *%d*\n"+
					"🧩 Comandos: *%d*\n"+
					"🔧 Go: *%s*",
				ctx.Config.NomeDoBot,
				uptime,
				float64(ms.Alloc)/1024/1024,
				runtime.NumGoroutine(),
				len(groups),
				len(ctx.Registry.AllUnique()),
				runtime.Version(),
			))
		},
	})

	r.Register(&Command{
		Name: "reiniciar", Aliases: []string{"restart"}, Category: "dono", Dono: true,
		Description: "Reinicia o processo do bot",
		Execute: func(ctx *Context) error {
			_ = ctx.Reply("🔄 Reiniciando bot...")
			go func() {
				time.Sleep(1 * time.Second)
				ctx.Bot.Restart()
			}()
			return nil
		},
	})
}

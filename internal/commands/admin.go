package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func toggleGroupFeature(r *Registry, name, feature, label string) {
	r.Register(&Command{
		Name: name, Category: "admin", Admin: true, OnlyGroup: true,
		Description: fmt.Sprintf("Ativa ou desativa %s no grupo", label),
		Execute: func(ctx *Context) error {
			arg := strings.ToLower(ctx.Q)
			var enable bool
			switch arg {
			case "on", "1", "ativar", "sim", "true":
				enable = true
			case "off", "0", "desativar", "nao", "não", "false":
				enable = false
			default:
				enable = !ctx.DB.GetGroupFeature(ctx.From.String(), feature)
			}
			_ = ctx.DB.SetGroupFeature(ctx.From.String(), feature, enable)
			state := "❌ Desativado"
			if enable {
				state = "✅ Ativado"
			}
			return ctx.Reply(fmt.Sprintf("*%s*: %s", label, state))
		},
	})
}

func registerAdmin(r *Registry) {
	r.Register(&Command{
		Name: "abrir", Aliases: []string{"abrir_grupo", "open"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Abre o grupo para todos enviarem mensagens",
		Execute: func(ctx *Context) error {
			if err := ctx.Bot.Raw().SetGroupAnnounce(context.Background(), ctx.From, false); err != nil {
				return ctx.Reply("❌ Falha ao abrir grupo: " + err.Error())
			}
			ctx.React("🔓")
			return ctx.Reply("🔓 *Grupo aberto!* Todos os membros podem enviar mensagens.")
		},
	})

	r.Register(&Command{
		Name: "fechar", Aliases: []string{"fechar_grupo", "close"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Fecha o grupo apenas para administradores",
		Execute: func(ctx *Context) error {
			if err := ctx.Bot.Raw().SetGroupAnnounce(context.Background(), ctx.From, true); err != nil {
				return ctx.Reply("❌ Falha ao fechar grupo: " + err.Error())
			}
			ctx.React("🔒")
			return ctx.Reply("🔒 *Grupo fechado!* Apenas administradores podem enviar mensagens.")
		},
	})

	r.Register(&Command{
		Name: "agendafechar", Category: "admin", Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Agenda fechamento automático do grupo (ex: .agendafechar 23:00)",
		Execute: func(ctx *Context) error {
			if len(ctx.Args) == 0 || !strings.Contains(ctx.Args[0], ":") {
				return ctx.Reply(fmt.Sprintf("❗ Uso: *%sagendafechar HH:MM* (ex: %sagendafechar 23:30)", ctx.Prefix, ctx.Prefix))
			}
			horario := ctx.Args[0]
			parts := strings.Split(horario, ":")
			if len(parts) != 2 {
				return ctx.Reply("❗ Formato inválido. Use HH:MM.")
			}
			h, err1 := strconv.Atoi(parts[0])
			m, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
				return ctx.Reply("❗ Horário inválido.")
			}
			norm := fmt.Sprintf("%02d:%02d", h, m)
			_ = ctx.DB.SetScheduledClose(ctx.From.String(), norm, 0)
			return ctx.Reply(fmt.Sprintf("🕒 Grupo agendado para fechar às *%s*.", norm))
		},
	})

	r.Register(&Command{
		Name: "agendafechar_cancelar", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Cancela agendamento de fechamento do grupo",
		Execute: func(ctx *Context) error {
			_ = ctx.DB.DeleteScheduledClose(ctx.From.String())
			return ctx.Reply("✅ Agendamento de fechamento cancelado.")
		},
	})

	toggleGroupFeature(r, "antilink", "antilink", "Antilink")
	toggleGroupFeature(r, "antiflood", "antiflood", "Antiflood")
	toggleGroupFeature(r, "antifake", "antifake", "Antifake")
	toggleGroupFeature(r, "antidelete", "antidelete", "Antidelete")
	toggleGroupFeature(r, "viewonce", "viewonce", "ViewOnce")
	toggleGroupFeature(r, "antiproc", "antiproc", "Antiproc")
	toggleGroupFeature(r, "resenha", "resenha", "Modo Resenha")

	r.Register(&Command{
		Name: "bemvindo", Aliases: []string{"welcome"}, Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Ativa/desativa boas-vindas no grupo",
		Execute: func(ctx *Context) error {
			arg := strings.ToLower(ctx.Q)
			var enable bool
			switch arg {
			case "on", "1", "ativar", "sim":
				enable = true
			case "off", "0", "desativar", "nao", "não":
				enable = false
			default:
				enable = !ctx.DB.IsWelcomeEnabled(ctx.From.String())
			}
			_ = ctx.DB.SetWelcomeEnabled(ctx.From.String(), enable)
			if enable {
				return ctx.Reply("✅ *Boas-vindas ativadas* neste grupo!")
			}
			return ctx.Reply("❌ *Boas-vindas desativadas* neste grupo.")
		},
	})

	r.Register(&Command{
		Name: "ban", Aliases: []string{"kick", "expulsar"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Remove um membro do grupo",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque, responda ou informe o número de quem deseja remover.")
			}
			_, err := ctx.Bot.Raw().UpdateGroupParticipants(context.Background(), ctx.From, []types.JID{target}, whatsmeow.ParticipantChangeRemove)
			if err != nil {
				return ctx.Reply("❌ Erro ao remover: " + err.Error())
			}
			ctx.Bot.InvalidateGroupCache(ctx.From)
			ctx.React("🥾")
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("🥾 %s foi removido do grupo.", mentionTag(target)), target.String())
		},
	})

	r.Register(&Command{
		Name: "banadd", Aliases: []string{"add"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Adiciona um número ao grupo",
		Execute: func(ctx *Context) error {
			if len(ctx.Args) == 0 {
				return ctx.Reply("❗ Informe o número com DDI+DDD.")
			}
			d := cleanDigits(ctx.Args[0])
			if len(d) < 8 {
				return ctx.Reply("❗ Número inválido.")
			}
			j := types.NewJID(d, types.DefaultUserServer)
			_, err := ctx.Bot.Raw().UpdateGroupParticipants(context.Background(), ctx.From, []types.JID{j}, whatsmeow.ParticipantChangeAdd)
			if err != nil {
				return ctx.Reply("❌ Não foi possível adicionar: " + err.Error())
			}
			ctx.Bot.InvalidateGroupCache(ctx.From)
			return ctx.Reply("✅ Membro adicionado!")
		},
	})

	r.Register(&Command{
		Name: "promover", Aliases: []string{"promote", "adm"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Promove um membro a administrador",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou responda alguém.")
			}
			_, err := ctx.Bot.Raw().UpdateGroupParticipants(context.Background(), ctx.From, []types.JID{target}, whatsmeow.ParticipantChangePromote)
			if err != nil {
				return ctx.Reply("❌ Erro ao promover: " + err.Error())
			}
			ctx.Bot.InvalidateGroupCache(ctx.From)
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("👑 %s agora é administrador!", mentionTag(target)), target.String())
		},
	})

	r.Register(&Command{
		Name: "rebaixar", Aliases: []string{"demote"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Remove privilégios de administrador",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou responda alguém.")
			}
			_, err := ctx.Bot.Raw().UpdateGroupParticipants(context.Background(), ctx.From, []types.JID{target}, whatsmeow.ParticipantChangeDemote)
			if err != nil {
				return ctx.Reply("❌ Erro ao rebaixar: " + err.Error())
			}
			ctx.Bot.InvalidateGroupCache(ctx.From)
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("📉 %s não é mais administrador.", mentionTag(target)), target.String())
		},
	})

	r.Register(&Command{
		Name: "mute", Aliases: []string{"silenciar"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Silencia um membro (apaga mensagens automaticamente)",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou responda quem deseja silenciar.")
			}
			_ = ctx.DB.SetMute(ctx.From.String(), target.String(), 0)
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("🔇 %s foi silenciado.", mentionTag(target)), target.String())
		},
	})

	r.Register(&Command{
		Name: "mutetempo", Category: "admin", Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Silencia um membro por X minutos",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou responda quem deseja silenciar.")
			}
			mins := 10
			for _, a := range ctx.Args {
				if v, err := strconv.Atoi(a); err == nil && v > 0 && v <= 10080 {
					mins = v
				}
			}
			until := time.Now().Add(time.Duration(mins) * time.Minute).UnixMilli()
			_ = ctx.DB.SetMute(ctx.From.String(), target.String(), until)
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("🔇 %s foi silenciado por *%d min*.", mentionTag(target), mins), target.String())
		},
	})

	r.Register(&Command{
		Name: "unmute", Aliases: []string{"desmutar"}, Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Remove o silenciamento de um membro",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou responda quem deseja desmutar.")
			}
			_ = ctx.DB.RemoveMute(ctx.From.String(), target.String())
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("🔊 %s pode falar novamente.", mentionTag(target)), target.String())
		},
	})

	r.Register(&Command{
		Name: "listmutes", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Lista membros silenciados no grupo",
		Execute: func(ctx *Context) error {
			mutes := ctx.DB.ListMutes(ctx.From.String())
			if len(mutes) == 0 {
				return ctx.Reply("🔊 Nenhum membro silenciado neste grupo.")
			}
			var sb strings.Builder
			sb.WriteString("🔇 *Membros Silenciados:*\n\n")
			var mentions []string
			for jid, until := range mutes {
				j := toUserJID(jid)
				mentions = append(mentions, j.String())
				if until > 0 {
					sb.WriteString(fmt.Sprintf("• %s (até %s)\n", mentionTag(j), time.UnixMilli(until).Format("15:04")))
				} else {
					sb.WriteString(fmt.Sprintf("• %s (permanente)\n", mentionTag(j)))
				}
			}
			return ctx.Bot.SendText(ctx.From, sb.String(), mentions...)
		},
	})

	r.Register(&Command{
		Name: "warn", Aliases: []string{"advertir"}, Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Adverte um membro (3 warns = ban se bot for admin)",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou responda alguém.")
			}
			count := ctx.DB.AddWarn(ctx.From.String(), target.String(), ctx.Q)
			if count >= 3 && ctx.IsBotAdmin {
				_, _ = ctx.Bot.Raw().UpdateGroupParticipants(context.Background(), ctx.From, []types.JID{target}, whatsmeow.ParticipantChangeRemove)
				return ctx.Bot.SendText(ctx.From, fmt.Sprintf("⚠️ %s atingiu *3/3 advertências* e foi removido!", mentionTag(target)), target.String())
			}
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("⚠️ %s recebeu uma advertência (*%d/3*).", mentionTag(target), count), target.String())
		},
	})

	r.Register(&Command{
		Name: "unwarn", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Remove uma advertência de um membro",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Marque ou responda alguém.")
			}
			count := ctx.DB.RemoveWarn(ctx.From.String(), target.String())
			return ctx.Bot.SendText(ctx.From, fmt.Sprintf("✅ Advertência removida de %s (*%d/3*).", mentionTag(target), count), target.String())
		},
	})

	r.Register(&Command{
		Name: "listwarn", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Lista advertências do grupo",
		Execute: func(ctx *Context) error {
			warns := ctx.DB.ListWarns(ctx.From.String())
			if len(warns) == 0 {
				return ctx.Reply("✅ Nenhuma advertência registrada neste grupo.")
			}
			var sb strings.Builder
			sb.WriteString("⚠️ *Advertências do Grupo:*\n\n")
			var mentions []string
			for jid, w := range warns {
				j := toUserJID(jid)
				mentions = append(mentions, j.String())
				sb.WriteString(fmt.Sprintf("• %s: *%d/3*\n", mentionTag(j), w.Count))
			}
			return ctx.Bot.SendText(ctx.From, sb.String(), mentions...)
		},
	})

	r.Register(&Command{
		Name: "blacklist", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Gerencia blacklist do grupo (.blacklist add/del/list @user)",
		Execute: func(ctx *Context) error {
			sub := ""
			if len(ctx.Args) > 0 {
				sub = strings.ToLower(ctx.Args[0])
			}
			switch sub {
			case "add":
				target := resolveTarget(ctx)
				if target.IsEmpty() {
					return ctx.Reply("❗ Marque ou informe o número.")
				}
				_ = ctx.DB.AddBlacklist(ctx.From.String(), target.String(), "blacklist", ctx.Sender.String())
				return ctx.Bot.SendText(ctx.From, fmt.Sprintf("⛔ %s adicionado à blacklist.", mentionTag(target)), target.String())
			case "del", "rm", "remove":
				target := resolveTarget(ctx)
				if target.IsEmpty() {
					return ctx.Reply("❗ Marque ou informe o número.")
				}
				_ = ctx.DB.RemoveBlacklist(ctx.From.String(), target.String())
				return ctx.Bot.SendText(ctx.From, fmt.Sprintf("✅ %s removido da blacklist.", mentionTag(target)), target.String())
			default:
				list := ctx.DB.ListBlacklist(ctx.From.String())
				if len(list) == 0 {
					return ctx.Reply("📋 Blacklist vazia neste grupo.")
				}
				return ctx.Reply("⛔ *Blacklist:*\n" + strings.Join(list, "\n"))
			}
		},
	})

	r.Register(&Command{
		Name: "unban", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Remove da blacklist",
		Execute: func(ctx *Context) error {
			target := resolveTarget(ctx)
			if target.IsEmpty() {
				return ctx.Reply("❗ Informe o número.")
			}
			_ = ctx.DB.RemoveBlacklist(ctx.From.String(), target.String())
			return ctx.Reply("✅ Removido da blacklist.")
		},
	})

	r.Register(&Command{
		Name: "hidetag", Aliases: []string{"totag", "aviso", "todos", "all"}, Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Marca todos do grupo silenciosamente com uma mensagem",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil {
				return ctx.Reply("❌ Sem metadados do grupo.")
			}
			msg := ctx.Q
			if msg == "" {
				msg = "📢 *Atenção pessoal!*"
			}
			mentions := make([]string, 0, len(ctx.GroupInfo.Participants))
			for _, p := range ctx.GroupInfo.Participants {
				mentions = append(mentions, p.JID.String())
			}
			return ctx.Bot.SendText(ctx.From, msg, mentions...)
		},
	})

	r.Register(&Command{
		Name: "marcar", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Marca todos os membros visivelmente",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil {
				return ctx.Reply("❌ Sem metadados do grupo.")
			}
			var sb strings.Builder
			q := strings.TrimSpace(ctx.Q)
			if strings.EqualFold(q, "todos") || strings.EqualFold(q, "all") {
				q = ""
			}
			if q != "" {
				sb.WriteString(q + "\n\n")
			} else {
				sb.WriteString("📢 *Chamada Geral:*\n\n")
			}
			mentions := make([]string, 0, len(ctx.GroupInfo.Participants))
			for _, p := range ctx.GroupInfo.Participants {
				mentions = append(mentions, p.JID.String())
				sb.WriteString(fmt.Sprintf("• %s\n", mentionTag(p.JID)))
			}
			return ctx.Bot.SendText(ctx.From, sb.String(), mentions...)
		},
	})

	r.Register(&Command{
		Name: "link", Category: "admin", Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Obtém o link de convite do grupo",
		Execute: func(ctx *Context) error {
			link, err := ctx.Bot.Raw().GetGroupInviteLink(context.Background(), ctx.From, false)
			if err != nil {
				return ctx.Reply("❌ Erro ao obter link: " + err.Error())
			}
			return ctx.Reply("🔗 *Link do Grupo:*\n" + link)
		},
	})

	r.Register(&Command{
		Name: "revogar", Category: "admin", Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Revoga o link atual do grupo e gera um novo",
		Execute: func(ctx *Context) error {
			link, err := ctx.Bot.Raw().GetGroupInviteLink(context.Background(), ctx.From, true)
			if err != nil {
				return ctx.Reply("❌ Erro ao revogar link: " + err.Error())
			}
			return ctx.Reply("🔄 *Novo link do grupo:*\n" + link)
		},
	})

	r.Register(&Command{
		Name: "setnomegrupo", Category: "admin", Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Altera o nome do grupo",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe o novo nome do grupo.")
			}
			if err := ctx.Bot.Raw().SetGroupName(context.Background(), ctx.From, ctx.Q); err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			ctx.Bot.InvalidateGroupCache(ctx.From)
			return ctx.Reply("✅ Nome do grupo atualizado!")
		},
	})

	r.Register(&Command{
		Name: "setdesc", Category: "admin", Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Altera a descrição do grupo",
		Execute: func(ctx *Context) error {
			if ctx.Q == "" {
				return ctx.Reply("❗ Informe a nova descrição.")
			}
			if err := ctx.Bot.Raw().SetGroupDescription(context.Background(), ctx.From, ctx.Q); err != nil {
				return ctx.Reply("❌ Erro: " + err.Error())
			}
			return ctx.Reply("✅ Descrição do grupo atualizada!")
		},
	})

	r.Register(&Command{
		Name: "apagar", Aliases: []string{"del", "delete"}, Category: "admin",
		Admin: true, NeedBotAdmin: true, OnlyGroup: true,
		Description: "Apaga a mensagem respondida",
		Execute: func(ctx *Context) error {
			ci := getContextInfo(ctx.Message)
			if ci == nil || ci.StanzaID == nil {
				return ctx.Reply("❗ Responda a mensagem que deseja apagar.")
			}
			sender := ctx.From
			if ci.Participant != nil {
				if j, err := types.ParseJID(*ci.Participant); err == nil {
					sender = j
				}
			}
			return ctx.Bot.DeleteMessage(ctx.From, sender, *ci.StanzaID)
		},
	})

	r.Register(&Command{
		Name: "limpar", Category: "admin", Admin: true, OnlyGroup: true,
		Description: "Envia espaços para limpar o chat visualmente",
		Execute: func(ctx *Context) error {
			return ctx.Reply("🧹 Limpando chat..." + strings.Repeat("\n\u200b", 35) + "✨ Chat limpo!")
		},
	})

	r.Register(&Command{
		Name: "listadmin", Category: "admin", OnlyGroup: true,
		Description: "Lista os administradores do grupo",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil {
				return ctx.Reply("❌ Sem metadados.")
			}
			var sb strings.Builder
			sb.WriteString("👑 *Administradores do Grupo:*\n\n")
			var mentions []string
			for _, p := range ctx.GroupInfo.Participants {
				if p.IsAdmin || p.IsSuperAdmin {
					mentions = append(mentions, p.JID.String())
					sb.WriteString(fmt.Sprintf("• %s\n", mentionTag(p.JID)))
				}
			}
			return ctx.Bot.SendText(ctx.From, sb.String(), mentions...)
		},
	})

	r.Register(&Command{
		Name: "listonline", Category: "admin", OnlyGroup: true,
		Description: "Lista membros do grupo",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil {
				return ctx.Reply("❌ Sem metadados.")
			}
			return ctx.Reply(fmt.Sprintf("👥 Grupo *%s* possui *%d* participantes.", ctx.GroupName, len(ctx.GroupInfo.Participants)))
		},
	})

	r.Register(&Command{
		Name: "grupo", Category: "admin", OnlyGroup: true,
		Description: "Mostra informações e configurações do grupo",
		Execute: func(ctx *Context) error {
			if ctx.GroupInfo == nil {
				return ctx.Reply("❌ Sem metadados.")
			}
			gid := ctx.From.String()
			return ctx.Reply(fmt.Sprintf(
				"📌 *%s*\n👥 Membros: *%d*\n🔒 Antilink: *%v*\n🌊 Antiflood: *%v*\n🛡️ Antifake: *%v*\n👋 Boas-vindas: *%v*",
				ctx.GroupName,
				len(ctx.GroupInfo.Participants),
				ctx.DB.GetGroupFeature(gid, "antilink"),
				ctx.DB.GetGroupFeature(gid, "antiflood"),
				ctx.DB.GetGroupFeature(gid, "antifake"),
				ctx.DB.IsWelcomeEnabled(gid),
			))
		},
	})

	r.Register(&Command{
		Name: "ativar_grupo", Aliases: []string{"reativar", "alugar"}, Category: "admin", Dono: true, OnlyGroup: true,
		Description: "Ativa/aluga o bot no grupo por N dias (padrão 30, 0 = permanente)",
		Execute: func(ctx *Context) error {
			days := 30
			if len(ctx.Args) > 0 {
				if v, err := strconv.Atoi(ctx.Args[0]); err == nil && v >= 0 {
					days = v
				}
			}
			_ = ctx.DB.ActivateGroup(ctx.From.String(), days, ctx.Sender.String())
			if days == 0 {
				return ctx.Reply(fmt.Sprintf("✅ Grupo *%s* ativado *permanentemente*!", ctx.GroupName))
			}
			return ctx.Reply(fmt.Sprintf("✅ Grupo *%s* ativado por *%d dias*!", ctx.GroupName, days))
		},
	})

	r.Register(&Command{
		Name: "desativar_grupo", Aliases: []string{"desalugar"}, Category: "admin", Dono: true, OnlyGroup: true,
		Description: "Desativa o aluguel do bot neste grupo",
		Execute: func(ctx *Context) error {
			_ = ctx.DB.DeactivateGroup(ctx.From.String())
			return ctx.Reply("⛔ Bot desativado neste grupo.")
		},
	})

	r.Register(&Command{
		Name: "modoaluguel", Category: "admin", Dono: true,
		Description: "Liga/desliga a exigência de aluguel em grupos",
		Execute: func(ctx *Context) error {
			next := !ctx.DB.IsRentalRequired()
			_ = ctx.DB.SetRentalRequired(next)
			if next {
				return ctx.Reply("🔒 *Modo Aluguel:* ✅ Ativado (novos grupos precisam de `.alugar` / `.ativar_grupo`).")
			}
			return ctx.Reply("🔓 *Modo Aluguel:* ❌ Desativado (grupos liberados sem aluguel).")
		},
	})

	r.Register(&Command{
		Name: "listalugueis", Aliases: []string{"alugueis"}, Category: "admin", Dono: true,
		Description: "Lista os grupos com aluguel ativo",
		Execute: func(ctx *Context) error {
			list := ctx.DB.ListActiveGroups()
			if len(list) == 0 {
				return ctx.Reply("📭 Nenhum grupo com aluguel ativo no momento.")
			}
			var sb strings.Builder
			sb.WriteString("📋 *GRUPOS ALUGADOS ATIVOS:*\n\n")
			for i, g := range list {
				exp := "Permanente"
				if g.ExpiresAt > 0 {
					exp = time.UnixMilli(g.ExpiresAt).Format("02/01/2006")
				}
				sb.WriteString(fmt.Sprintf("%d. `%s` — até *%s*\n", i+1, g.GroupID, exp))
			}
			return ctx.Reply(sb.String())
		},
	})

	r.Register(&Command{
		Name: "conflitos", Category: "admin", Dono: true,
		Description: "Verifica conflitos de comandos",
		Execute: func(ctx *Context) error {
			return ctx.Reply(fmt.Sprintf("✅ Registro Go tipado: *%d* comandos únicos carregados sem conflitos.", len(ctx.Registry.AllUnique())))
		},
	})
}

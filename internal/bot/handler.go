package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nyxbot-go/internal/commands"
	"nyxbot-go/internal/config"
	"nyxbot-go/internal/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var linkRE = regexp.MustCompile(`(?i)(https?://[^\s]+)|(www\.[^\s]+)|(wa\.me/[^\s]+)|(chat\.whatsapp\.com/[^\s]+)`)

func (c *Client) seenMessage(id string) bool {
	if id == "" {
		return false
	}
	now := time.Now()
	c.seenMu.Lock()
	defer c.seenMu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = now
	if len(c.seen) > 1000 {
		for k, t := range c.seen {
			if now.Sub(t) > time.Minute {
				delete(c.seen, k)
			}
		}
	}
	return false
}

func unwrapMsg(m *waE2E.Message) *waE2E.Message {
	if m == nil {
		return nil
	}
	cur := m
	for i := 0; i < 6; i++ {
		var next *waE2E.Message
		switch {
		case cur.EphemeralMessage != nil && cur.EphemeralMessage.Message != nil:
			next = cur.EphemeralMessage.Message
		case cur.ViewOnceMessage != nil && cur.ViewOnceMessage.Message != nil:
			next = cur.ViewOnceMessage.Message
		case cur.ViewOnceMessageV2 != nil && cur.ViewOnceMessageV2.Message != nil:
			next = cur.ViewOnceMessageV2.Message
		case cur.ViewOnceMessageV2Extension != nil && cur.ViewOnceMessageV2Extension.Message != nil:
			next = cur.ViewOnceMessageV2Extension.Message
		case cur.DocumentWithCaptionMessage != nil && cur.DocumentWithCaptionMessage.Message != nil:
			next = cur.DocumentWithCaptionMessage.Message
		case cur.EditedMessage != nil && cur.EditedMessage.Message != nil:
			next = cur.EditedMessage.Message
		}
		if next == nil || next == cur {
			break
		}
		cur = next
	}
	return cur
}

func extractBody(m *waE2E.Message) string {
	u := unwrapMsg(m)
	if u == nil {
		return ""
	}
	switch {
	case u.Conversation != nil:
		return *u.Conversation
	case u.ExtendedTextMessage != nil && u.ExtendedTextMessage.Text != nil:
		return *u.ExtendedTextMessage.Text
	case u.ImageMessage != nil && u.ImageMessage.Caption != nil:
		return *u.ImageMessage.Caption
	case u.VideoMessage != nil && u.VideoMessage.Caption != nil:
		return *u.VideoMessage.Caption
	case u.DocumentMessage != nil && u.DocumentMessage.Caption != nil:
		return *u.DocumentMessage.Caption
	case u.ButtonsResponseMessage != nil && u.ButtonsResponseMessage.GetSelectedDisplayText() != "":
		return u.ButtonsResponseMessage.GetSelectedDisplayText()
	case u.ListResponseMessage != nil && u.ListResponseMessage.Title != nil:
		return *u.ListResponseMessage.Title
	case u.TemplateButtonReplyMessage != nil && u.TemplateButtonReplyMessage.SelectedDisplayText != nil:
		return *u.TemplateButtonReplyMessage.SelectedDisplayText
	case u.GroupInviteMessage != nil && u.GroupInviteMessage.Caption != nil:
		return *u.GroupInviteMessage.Caption
	}
	return ""
}

func hasLink(m *waE2E.Message, body string) bool {
	if linkRE.MatchString(body) {
		return true
	}
	u := unwrapMsg(m)
	if u == nil {
		return false
	}
	if u.GroupInviteMessage != nil {
		return true
	}
	if u.ExtendedTextMessage != nil && u.ExtendedTextMessage.MatchedText != nil {
		if linkRE.MatchString(*u.ExtendedTextMessage.MatchedText) {
			return true
		}
	}
	return false
}

func (c *Client) checkFlood(groupID, userID string) bool {
	key := groupID + ":" + userID
	now := time.Now()
	c.floodMu.Lock()
	defer c.floodMu.Unlock()
	times := c.flood[key]
	var fresh []time.Time
	for _, t := range times {
		if now.Sub(t) < 8*time.Second {
			fresh = append(fresh, t)
		}
	}
	fresh = append(fresh, now)
	c.flood[key] = fresh
	return len(fresh) >= 6
}

func (c *Client) handleMessage(evt *events.Message) {
	if evt == nil || evt.Message == nil {
		return
	}
	if c.seenMessage(evt.Info.ID) {
		return
	}
	chat := evt.Info.Chat
	if chat.String() == "status@broadcast" || chat.Server == types.NewsletterServer {
		return
	}

	isGroup := chat.Server == types.GroupServer
	sender := evt.Info.Sender.ToNonAD()
	permSender := c.ResolvePN(sender)
	if evt.Info.SenderAlt.User != "" && sender.Server == types.HiddenUserServer {
		permSender = evt.Info.SenderAlt.ToNonAD()
	}

	prefix := c.cfg.Prefix
	if isGroup {
		if gp := c.db.GetGroupPrefix(chat.String()); gp != "" {
			prefix = gp
		}
	}

	body := strings.TrimSpace(extractBody(evt.Message))
	if evt.Info.IsFromMe && !strings.HasPrefix(body, prefix) {
		return
	}

	// Dono check
	donoNum := config.CleanNumber(c.cfg.NumeroDoDono)
	senderNum := config.CleanNumber(permSender.User)
	botNum := ""
	if c.wa.Store.ID != nil {
		botNum = config.CleanNumber(c.wa.Store.ID.User)
	}
	isDono := (donoNum != "" && senderNum == donoNum) ||
		(botNum != "" && senderNum == botNum) ||
		c.db.IsDonoExtra(permSender.String()) ||
		c.db.IsDonoExtra(sender.String())

	isVip := c.db.IsPremium(permSender.String()) || c.db.IsPremium(sender.String())

	var groupInfo *types.GroupInfo
	var groupName string
	var isAdmin, isSuperAdmin, isBotAdmin bool
	if isGroup {
		if gi, err := c.GetGroupInfo(context.Background(), chat); err == nil && gi != nil {
			groupInfo = gi
			groupName = gi.Name
			for _, p := range gi.Participants {
				pPN := c.ResolvePN(p.JID)
				if p.JID.ToNonAD() == sender || pPN == permSender {
					if p.IsSuperAdmin {
						isSuperAdmin = true
						isAdmin = true
					} else if p.IsAdmin {
						isAdmin = true
					}
				}
				if c.wa.Store.ID != nil && (p.JID.User == c.wa.Store.ID.User || pPN.User == c.wa.Store.ID.User) {
					if p.IsAdmin || p.IsSuperAdmin {
						isBotAdmin = true
					}
				}
			}
		}
	}
	if isDono {
		isAdmin = true
	}

	// AFK return check
	if afk := c.db.GetAFK(permSender.String()); afk != nil && !strings.HasPrefix(body, prefix+"afk") {
		_ = c.db.ClearAFK(permSender.String())
		_ = c.ReplyText(evt, fmt.Sprintf("👋 Bem-vindo(a) de volta! Você estava AFK desde %s.", time.UnixMilli(afk.SinceTS).Format("15:04")))
	}

	// Group passive protections
	if isGroup && !isAdmin {
		gid := chat.String()

		// Mute check
		if c.db.IsMuted(gid, permSender.String()) || c.db.IsMuted(gid, sender.String()) {
			if isBotAdmin {
				_ = c.DeleteMessage(chat, evt.Info.Sender, evt.Info.ID)
			}
			return
		}

		// Antilink check
		if c.db.GetGroupFeature(gid, "antilink") && hasLink(evt.Message, body) {
			if isBotAdmin {
				if err := c.DeleteMessage(chat, evt.Info.Sender, evt.Info.ID); err == nil {
					_ = c.SendText(chat, "🔗 Link removido (antilink).")
				}
			}
			return
		}

		// Antiflood check
		if c.db.GetGroupFeature(gid, "antiflood") && c.checkFlood(gid, permSender.String()) {
			if isBotAdmin {
				_ = c.DeleteMessage(chat, evt.Info.Sender, evt.Info.ID)
			}
			_ = c.SendText(chat, fmt.Sprintf("🌊 %s, devagar com o flood!", "@"+permSender.User), permSender.String())
			return
		}

		// Autoreply check
		if body != "" {
			lower := strings.ToLower(body)
			for trig, resp := range c.db.GetAutoreplies(gid) {
				if trig != "" && strings.Contains(lower, trig) {
					_ = c.ReplyText(evt, resp)
					break
				}
			}
		}
	}

	// Non-command XP & Lunar natural-language commands / human-like persona reply
	if !strings.HasPrefix(body, prefix) {
		if body != "" {
			c.db.AddXP(permSender.String(), c.cfg.Moeda.XpPorMensagem, c.cfg.Niveis.XpBase, c.cfg.Niveis.Multiplicador)
		}
		if c.isLunarTriggered(evt, body) {
			if cmdName, args, q, ok := c.parseNaturalCommand(body); ok {
				if cmd := c.registry.Get(cmdName); cmd != nil {
					c.executeCommand(evt, cmd, chat, permSender, isGroup, isDono, isAdmin, isSuperAdmin, isBotAdmin, isVip, groupInfo, groupName, prefix, cmdName, args, q, body)
					return
				}
			}
			c.replyAsLunar(evt, evt.Info.PushName, body, isDono)
		}
		return
	}

	withoutPrefix := strings.TrimSpace(strings.TrimPrefix(body, prefix))
	var cmdName, q string
	var args []string
	if withoutPrefix == "" && body == prefix {
		cmdName = "."
	} else {
		fields := strings.Fields(withoutPrefix)
		if len(fields) > 0 {
			cmdName = strings.ToLower(fields[0])
			args = fields[1:]
			q = strings.Join(args, " ")
		}
	}

	cmd := c.registry.Get(cmdName)
	if cmd == nil {
		return
	}

	c.executeCommand(evt, cmd, chat, permSender, isGroup, isDono, isAdmin, isSuperAdmin, isBotAdmin, isVip, groupInfo, groupName, prefix, cmdName, args, q, body)
}

func (c *Client) executeCommand(
	evt *events.Message,
	cmd *commands.Command,
	chat, permSender types.JID,
	isGroup, isDono, isAdmin, isSuperAdmin, isBotAdmin, isVip bool,
	groupInfo *types.GroupInfo,
	groupName, prefix, cmdName string,
	args []string,
	q, body string,
) {

	ctx := &commands.Context{
		Bot:          c,
		DB:           c.db,
		Config:       c.cfg,
		Registry:     c.registry,
		Event:        evt,
		Message:      unwrapMsg(evt.Message),
		From:         chat,
		Sender:       permSender,
		PermSender:   permSender,
		PushName:     evt.Info.PushName,
		IsGroup:      isGroup,
		IsDono:       isDono,
		IsAdmin:      isAdmin,
		IsSuperAdmin: isSuperAdmin,
		IsBotAdmin:   isBotAdmin,
		IsVip:        isVip,
		GroupInfo:    groupInfo,
		GroupName:    groupName,
		Prefix:       prefix,
		CmdName:      cmdName,
		Args:         args,
		Q:            q,
		Body:         body,
	}

	// Group active (aluguel) check
	if isGroup && !isDono && !isVip {
		allowAlways := map[string]bool{
			"dono": true, "quemdono": true, "meuid": true, "ping": true,
			"menu": true, "ajuda": true, "help": true, "commands": true,
			"comandos": true, "cantada": true, "ativar_grupo": true,
		}
		if !allowAlways[cmd.Name] && !c.db.IsGroupActive(chat.String()) {
			_ = ctx.Reply(fmt.Sprintf("⛔ *Bot não ativo neste grupo.*\nPeça ao dono para ativar com *%sativar_grupo*.", prefix))
			return
		}
	}

	if ok, reason := c.registry.CanRun(cmd, ctx); !ok {
		_ = ctx.Reply(reason)
		return
	}

	logger.Command(evt.Info.PushName, groupName, prefix+cmdName, isGroup)

	leveled, newLvl := c.db.AddXP(permSender.String(), c.cfg.Moeda.XpPorComando, c.cfg.Niveis.XpBase, c.cfg.Niveis.Multiplicador)
	if leveled {
		_ = ctx.Reply(fmt.Sprintf("✨ Você subiu para o nível *%d*!", newLvl))
	}

	_ = c.wa.SendChatPresence(context.Background(), chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	if err := cmd.Execute(ctx); err != nil {
		logger.Red("Erro no comando %s: %v", cmdName, err)
		_ = ctx.Reply(fmt.Sprintf("❌ Erro ao executar o comando.\n`%v`", err))
	}
	_ = c.wa.SendChatPresence(context.Background(), chat, types.ChatPresencePaused, types.ChatPresenceMediaText)
}

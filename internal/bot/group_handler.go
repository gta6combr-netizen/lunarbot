package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"nyxbot-go/internal/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const welcomeCaption = `𝗦𝗘𝗝𝗔 𝗠𝗨𝗜𝗧𝗢 𝗕𝗘𝗠 𝗩𝗜𝗡𝗗𝗢 (𝗔̀) 𝗔̀𝗢 𝗟𝗔𝗧𝗔𝗠 ! 💙🖤

Você passou no teste e agora veste o manto da guilda. Aqui dentro não tem moleza é treino, call, e é decidir a partida quando ela aperta.
Se adapta rápido, escuta o time, e *lembre-se:* na Tam a gente joga junto, evolui junto e cobra junto. Errou? Treina. Acertou? Repete até virar padrão. Patente não define player, jogabilidade e visão de jogo define. Mostra porque a gente abriu vaga pra você. Bora subir de nível e levar o nome Latam pro topo!`

var (
	welcomeMu  sync.Mutex
	welcomeLog = make(map[string]time.Time)
)

func (c *Client) handleGroupInfo(evt *events.GroupInfo) {
	if evt == nil || evt.JID.IsEmpty() {
		return
	}
	c.InvalidateGroupCache(evt.JID)

	if len(evt.Join) == 0 {
		return
	}

	gid := evt.JID.String()
	welcomeOn := c.db.IsWelcomeEnabled(gid)
	antifakeOn := c.db.GetGroupFeature(gid, "antifake")

	for _, jid := range evt.Join {
		pn := c.ResolvePN(jid)

		// Blacklist check
		if c.db.IsBlacklisted(gid, pn.String()) || c.db.IsBlacklisted(gid, jid.String()) {
			logger.Yellow("⛔ Blacklist: removendo %s de %s", pn.String(), gid)
			_, _ = c.wa.UpdateGroupParticipants(context.Background(), evt.JID, []types.JID{jid}, whatsmeow.ParticipantChangeRemove)
			_ = c.SendText(evt.JID, fmt.Sprintf("⛔ @%s está na blacklist e foi removido.", pn.User), jid.String())
			continue
		}

		// Antifake check (non-Brazilian +55 numbers)
		if antifakeOn && pn.Server == types.DefaultUserServer && !strings.HasPrefix(pn.User, "55") {
			logger.Yellow("🛡️ Antifake: removendo %s de %s", pn.String(), gid)
			_, _ = c.wa.UpdateGroupParticipants(context.Background(), evt.JID, []types.JID{jid}, whatsmeow.ParticipantChangeRemove)
			continue
		}

		if welcomeOn {
			c.sendWelcome(evt.JID, jid)
		}
	}
}

func (c *Client) sendWelcome(groupID, participant types.JID) {
	key := groupID.String() + ":" + participant.String()
	welcomeMu.Lock()
	if t, ok := welcomeLog[key]; ok && time.Since(t) < time.Minute {
		welcomeMu.Unlock()
		return
	}
	welcomeLog[key] = time.Now()
	welcomeMu.Unlock()

	text := fmt.Sprintf("@%s %s", participant.User, welcomeCaption)
	_ = c.SendText(groupID, text, participant.String())
}

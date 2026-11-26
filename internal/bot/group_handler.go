package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nyxbot-go/internal/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const welcomeCaption = `🎰 *BEM-VINDO A LAS VEGAS* 🎰
╰──────────────────────────╯
💸 *A sorte está do seu lado.*
🎲 *Vamos jogar?*
▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬
📌 *Para começar, se apresente:*
🃏 Foto ou vídeo
👤 Nome
🎂 Idade
📍 Cidade
▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬
💰 *A mesa está aberta.*
♠️ *Lunar Bot te dá as boas-vindas.*`

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

	var imgBytes []byte
	if pic, err := c.wa.GetProfilePictureInfo(context.Background(), participant, &whatsmeow.GetProfilePictureParams{}); err == nil && pic != nil && pic.URL != "" {
		if resp, err := http.Get(pic.URL); err == nil {
			imgBytes, _ = io.ReadAll(io.LimitReader(resp.Body, 5<<20))
			resp.Body.Close()
		}
	}
	if len(imgBytes) == 0 {
		defPath := filepath.Join(c.cfg.RootDir, "assets", "welcome-default.jpeg")
		imgBytes, _ = os.ReadFile(defPath)
	}

	caption := fmt.Sprintf("@%s\n\n%s", participant.User, welcomeCaption)
	if len(imgBytes) > 0 {
		_ = c.SendImage(groupID, imgBytes, caption, nil, participant.String())
	} else {
		_ = c.SendText(groupID, caption, participant.String())
	}
}

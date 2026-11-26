package commands

import (
	"context"
	"nyxbot-go/internal/config"
	"nyxbot-go/internal/db"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type BotAPI interface {
	Raw() *whatsmeow.Client
	ReplyText(evt *events.Message, text string) error
	SendText(to types.JID, text string, mentions ...string) error
	React(evt *events.Message, emoji string)
	SendImage(to types.JID, data []byte, caption string, quoted *events.Message, mentions ...string) error
	SendVideo(to types.JID, data []byte, caption string, gifPlayback bool, quoted *events.Message, mentions ...string) error
	SendAudio(to types.JID, data []byte, ptt bool, quoted *events.Message) error
	SendSticker(to types.JID, webpData []byte, quoted *events.Message) error
	SendDocument(to types.JID, data []byte, filename, mimetype, caption string, quoted *events.Message) error
	DeleteMessage(chat types.JID, sender types.JID, msgID types.MessageID) error
	GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error)
	InvalidateGroupCache(jid types.JID)
	ResolvePN(jid types.JID) types.JID
	Restart()
}

type Context struct {
	Bot          BotAPI
	DB           *db.DB
	Config       *config.Config
	Registry     *Registry
	Event        *events.Message
	Message      *waE2E.Message
	From         types.JID
	Sender       types.JID
	PermSender   types.JID
	PushName     string
	IsGroup      bool
	IsDono       bool
	IsAdmin      bool
	IsSuperAdmin bool
	IsBotAdmin   bool
	IsVip        bool
	GroupInfo    *types.GroupInfo
	GroupName    string
	Prefix       string
	CmdName      string
	Args         []string
	Q            string
	Body         string
}

func (c *Context) Reply(text string) error {
	return c.Bot.ReplyText(c.Event, text)
}

func (c *Context) React(emoji string) {
	c.Bot.React(c.Event, emoji)
}

func (c *Context) SendImage(data []byte, caption string, mentions ...string) error {
	return c.Bot.SendImage(c.From, data, caption, c.Event, mentions...)
}

func (c *Context) SendVideo(data []byte, caption string, gif bool, mentions ...string) error {
	return c.Bot.SendVideo(c.From, data, caption, gif, c.Event, mentions...)
}

func (c *Context) SendSticker(webp []byte) error {
	return c.Bot.SendSticker(c.From, webp, c.Event)
}

func (c *Context) SendAudio(data []byte, ptt bool) error {
	return c.Bot.SendAudio(c.From, data, ptt, c.Event)
}

type Command struct {
	Name         string
	Aliases      []string
	Category     string
	Description  string
	Usage        string
	Cooldown     int
	Dono         bool
	Admin        bool
	NeedBotAdmin bool
	Premium      bool
	OnlyPv       bool
	OnlyGroup    bool
	Execute      func(ctx *Context) error
}

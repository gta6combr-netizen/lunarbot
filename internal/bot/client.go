package bot

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nyxbot-go/internal/commands"
	"nyxbot-go/internal/config"
	"nyxbot-go/internal/db"
	"nyxbot-go/internal/logger"

	"github.com/mdp/qrterminal/v3"
	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

type groupCacheEntry struct {
	info      *types.GroupInfo
	expiresAt time.Time
}

type Client struct {
	wa       *whatsmeow.Client
	db       *db.DB
	cfg      *config.Config
	registry *commands.Registry

	qrMu        sync.RWMutex
	currentQR   []byte
	isConnected bool

	groupMu    sync.RWMutex
	groupCache map[types.JID]groupCacheEntry

	seenMu sync.Mutex
	seen   map[string]time.Time

	floodMu sync.Mutex
	flood   map[string][]time.Time

	restartFn func()
}

func NewClient(cfg *config.Config, database *db.DB, reg *commands.Registry, useCode bool, restartFn func()) (*Client, error) {
	ctx := context.Background()
	sessionDB := filepath.Join(cfg.RootDir, "database", "whatsmeow.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", sessionDB)

	container, err := sqlstore.New(ctx, "sqlite", dsn, waLog.Noop)
	if err != nil {
		return nil, err
	}
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, err
	}

	waClient := whatsmeow.NewClient(deviceStore, waLog.Noop)
	c := &Client{
		wa:         waClient,
		db:         database,
		cfg:        cfg,
		registry:   reg,
		groupCache: make(map[types.JID]groupCacheEntry),
		seen:       make(map[string]time.Time),
		flood:      make(map[string][]time.Time),
		restartFn:  restartFn,
	}

	waClient.AddEventHandler(c.handleEvent)

	if waClient.Store.ID == nil {
		if useCode {
			phone := config.CleanNumber(cfg.NumeroDoDono)
			if phone == "" {
				return nil, fmt.Errorf("NumeroDoDono não configurado para --code")
			}
			if err := waClient.Connect(); err != nil {
				return nil, err
			}
			time.Sleep(2 * time.Second)
			code, err := waClient.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
			if err != nil {
				return nil, err
			}
			logger.Green("📱 Código de pareamento: %s", code)
		} else {
			qrChan, _ := waClient.GetQRChannel(ctx)
			if err := waClient.Connect(); err != nil {
				return nil, err
			}
			go func() {
				for evt := range qrChan {
					if evt.Event == "code" {
						logger.Yellow("📷 Escaneie o QR Code:")
						qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
						if png, err := qrcode.Encode(evt.Code, qrcode.Medium, 300); err == nil {
							c.qrMu.Lock()
							c.currentQR = png
							c.qrMu.Unlock()
						}
					} else {
						logger.Cyan("QR event: %s", evt.Event)
					}
				}
			}()
		}
	} else {
		if err := waClient.Connect(); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Client) Disconnect() {
	c.wa.Disconnect()
}

func (c *Client) Raw() *whatsmeow.Client {
	return c.wa
}

func (c *Client) Restart() {
	if c.restartFn != nil {
		c.restartFn()
	}
}

func (c *Client) SynthesizeVoice(text string) ([]byte, error) {
	return SynthesizeVoice(c.cfg.RootDir, c.cfg.ElevenLabsAPIKey, c.cfg.ElevenLabsVoiceID, c.cfg.GeminiAPIKey, text)
}

func (c *Client) QRStatus() (connected bool, qrPNG []byte) {
	c.qrMu.RLock()
	defer c.qrMu.RUnlock()
	return c.isConnected, c.currentQR
}

func (c *Client) handleEvent(rawEvt any) {
	switch evt := rawEvt.(type) {
	case *events.Connected:
		c.qrMu.Lock()
		c.isConnected = true
		c.currentQR = nil
		c.qrMu.Unlock()
		id := "?"
		if c.wa.Store.ID != nil {
			id = c.wa.Store.ID.String()
		}
		logger.Green("✅ Conectado como %s", id)
	case *events.Disconnected:
		c.qrMu.Lock()
		c.isConnected = false
		c.qrMu.Unlock()
		logger.Yellow("⚠️ Desconectado, reconectando...")
	case *events.LoggedOut:
		c.qrMu.Lock()
		c.isConnected = false
		c.qrMu.Unlock()
		logger.Red("❌ Sessão desconectada (LoggedOut).")
	case *events.Message:
		go c.handleMessage(evt)
	case *events.GroupInfo:
		go c.handleGroupInfo(evt)
	}
}

func (c *Client) ResolvePN(jid types.JID) types.JID {
	if jid.Server == types.HiddenUserServer {
		if pn, err := c.wa.Store.LIDs.GetPNForLID(context.Background(), jid); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD()
		}
	}
	return jid.ToNonAD()
}

func (c *Client) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	c.groupMu.RLock()
	entry, ok := c.groupCache[jid]
	c.groupMu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.info, nil
	}
	info, err := c.wa.GetGroupInfo(ctx, jid)
	if err != nil {
		return nil, err
	}
	c.groupMu.Lock()
	c.groupCache[jid] = groupCacheEntry{info: info, expiresAt: time.Now().Add(5 * time.Minute)}
	c.groupMu.Unlock()
	return info, nil
}

func (c *Client) InvalidateGroupCache(jid types.JID) {
	c.groupMu.Lock()
	delete(c.groupCache, jid)
	c.groupMu.Unlock()
}

func (c *Client) quotedContext(evt *events.Message, mentions []string) *waE2E.ContextInfo {
	if evt == nil && len(mentions) == 0 {
		return nil
	}
	ci := &waE2E.ContextInfo{}
	if len(mentions) > 0 {
		ci.MentionedJID = mentions
	}
	if evt != nil {
		ci.StanzaID = proto.String(evt.Info.ID)
		ci.Participant = proto.String(evt.Info.Sender.ToNonAD().String())
		ci.QuotedMessage = evt.Message
	}
	return ci
}

func (c *Client) ReplyText(evt *events.Message, text string) error {
	_, err := c.wa.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: c.quotedContext(evt, nil),
		},
	})
	return err
}

func (c *Client) SendText(to types.JID, text string, mentions ...string) error {
	msg := &waE2E.Message{}
	if len(mentions) > 0 {
		msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: &waE2E.ContextInfo{MentionedJID: mentions},
		}
	} else {
		msg.Conversation = proto.String(text)
	}
	_, err := c.wa.SendMessage(context.Background(), to, msg)
	return err
}

func (c *Client) React(evt *events.Message, emoji string) {
	if evt == nil {
		return
	}
	_, _ = c.wa.SendMessage(context.Background(), evt.Info.Chat, c.wa.BuildReaction(evt.Info.Chat, evt.Info.Sender, evt.Info.ID, emoji))
}

func (c *Client) SendImage(to types.JID, data []byte, caption string, quoted *events.Message, mentions ...string) error {
	up, err := c.wa.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	mime := http.DetectContentType(data)
	_, err = c.wa.SendMessage(context.Background(), to, &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mime),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			ContextInfo:   c.quotedContext(quoted, mentions),
		},
	})
	return err
}

func (c *Client) SendVideo(to types.JID, data []byte, caption string, gifPlayback bool, quoted *events.Message, mentions ...string) error {
	up, err := c.wa.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}
	_, err = c.wa.SendMessage(context.Background(), to, &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String("video/mp4"),
			GifPlayback:   proto.Bool(gifPlayback),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			ContextInfo:   c.quotedContext(quoted, mentions),
		},
	})
	return err
}

func (c *Client) SendAudio(to types.JID, data []byte, ptt bool, quoted *events.Message) error {
	return c.SendAudioWithCard(to, data, ptt, "", "", "", "", nil, quoted)
}

func (c *Client) SendAudioWithCard(to types.JID, data []byte, ptt bool, title, body, sourceURL, thumbURL string, thumb []byte, quoted *events.Message) error {
	up, err := c.wa.Upload(context.Background(), data, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}
	mime := "audio/mpeg"
	if ptt {
		mime = "audio/ogg; codecs=opus"
	}
	ci := c.quotedContext(quoted, nil)
	if title != "" || body != "" || len(thumb) > 0 {
		if ci == nil {
			ci = &waE2E.ContextInfo{}
		}
		ci.ExternalAdReply = &waE2E.ContextInfo_ExternalAdReplyInfo{
			Title:                 proto.String(title),
			Body:                  proto.String(body),
			MediaType:             waE2E.ContextInfo_ExternalAdReplyInfo_IMAGE.Enum(),
			Thumbnail:             thumb,
			ThumbnailURL:          proto.String(thumbURL),
			SourceURL:             proto.String(sourceURL),
			RenderLargerThumbnail: proto.Bool(true),
		}
	}
	_, err = c.wa.SendMessage(context.Background(), to, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			Mimetype:      proto.String(mime),
			PTT:           proto.Bool(ptt),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			ContextInfo:   ci,
		},
	})
	return err
}

func (c *Client) SendSticker(to types.JID, webpData []byte, quoted *events.Message) error {
	up, err := c.wa.Upload(context.Background(), webpData, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	_, err = c.wa.SendMessage(context.Background(), to, &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			Mimetype:      proto.String("image/webp"),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(webpData))),
			ContextInfo:   c.quotedContext(quoted, nil),
		},
	})
	return err
}

func (c *Client) SendStickerWithAudio(to types.JID, webpData, audioData []byte, isAnimated bool, quoted *events.Message) error {
	upSticker, err := c.wa.Upload(context.Background(), webpData, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	upAudio, err := c.wa.Upload(context.Background(), audioData, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}
	_, err = c.wa.SendMessage(context.Background(), to, &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			Mimetype:      proto.String("image/webp"),
			Width:         proto.Uint32(512),
			Height:        proto.Uint32(512),
			IsAnimated:    proto.Bool(isAnimated),
			URL:           proto.String(upSticker.URL),
			DirectPath:    proto.String(upSticker.DirectPath),
			MediaKey:      upSticker.MediaKey,
			FileEncSHA256: upSticker.FileEncSHA256,
			FileSHA256:    upSticker.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(webpData))),
			ContextInfo:   c.quotedContext(quoted, nil),
			Audio: &waE2E.StickerMessage_AudioMessage{
				AudioMessage: &waE2E.AudioMessage{
					Mimetype:      proto.String("audio/ogg; codecs=opus"),
					URL:           proto.String(upAudio.URL),
					DirectPath:    proto.String(upAudio.DirectPath),
					MediaKey:      upAudio.MediaKey,
					FileEncSHA256: upAudio.FileEncSHA256,
					FileSHA256:    upAudio.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(audioData))),
				},
			},
		},
	})
	return err
}

func (c *Client) SendDocument(to types.JID, data []byte, filename, mimetype, caption string, quoted *events.Message) error {
	up, err := c.wa.Upload(context.Background(), data, whatsmeow.MediaDocument)
	if err != nil {
		return err
	}
	_, err = c.wa.SendMessage(context.Background(), to, &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			FileName:      proto.String(filename),
			Mimetype:      proto.String(mimetype),
			Caption:       proto.String(caption),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			ContextInfo:   c.quotedContext(quoted, nil),
		},
	})
	return err
}

func (c *Client) DeleteMessage(chat types.JID, sender types.JID, msgID types.MessageID) error {
	_, err := c.wa.SendMessage(context.Background(), chat, c.wa.BuildRevoke(chat, sender, msgID))
	return err
}

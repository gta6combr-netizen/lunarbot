package bot

import (
	"nyxbot-go/internal/logger"

	"go.mau.fi/whatsmeow/types/events"
)

const (
	queueBufferSize = 128
	queueWorkers    = 3
)

func (c *Client) initQueue() {
	c.msgQueue = make(chan *events.Message, queueBufferSize)
	for i := 0; i < queueWorkers; i++ {
		go c.runWorker()
	}
}

func (c *Client) runWorker() {
	for evt := range c.msgQueue {
		c.safeHandleMessage(evt)
	}
}

func (c *Client) safeHandleMessage(evt *events.Message) {
	defer func() {
		if r := recover(); r != nil {
			logger.Red("[Queue] panic recuperado: %v", r)
		}
	}()
	c.handleMessage(evt)
}

func (c *Client) enqueueMessage(evt *events.Message) {
	if c.msgQueue == nil {
		go c.safeHandleMessage(evt)
		return
	}
	select {
	case c.msgQueue <- evt:
	default:
		logger.Yellow("[Queue] fila cheia (%d), descartando mensagem %s", queueBufferSize, evt.Info.ID)
	}
}

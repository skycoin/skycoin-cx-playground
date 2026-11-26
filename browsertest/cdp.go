package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/coder/websocket"
)

// cdpConnection is one Chrome DevTools Protocol connection to a page target.
type cdpConnection struct {
	conn    *websocket.Conn
	mutex   sync.Mutex
	nextID  int64
	waiting map[int64]chan cdpReply
	closed  chan struct{}
	readErr error
}

type cdpReply struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type cdpMessage struct {
	ID int64 `json:"id"`
	cdpReply
}

// cdpConnect dials the page's webSocketDebuggerUrl.
func cdpConnect(ctx context.Context, url string) (*cdpConnection, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(64 << 20)
	c := &cdpConnection{conn: conn, waiting: map[int64]chan cdpReply{}, closed: make(chan struct{})}
	go c.readLoop()
	return c, nil
}

func (c *cdpConnection) readLoop() {
	defer close(c.closed)
	for {
		_, b, err := c.conn.Read(context.Background())
		if err != nil {
			c.mutex.Lock()
			c.readErr = err
			c.mutex.Unlock()
			return
		}
		var m cdpMessage
		if json.Unmarshal(b, &m) != nil || m.ID == 0 {
			continue // an event, which the steps do not use
		}
		c.mutex.Lock()
		ch := c.waiting[m.ID]
		delete(c.waiting, m.ID)
		c.mutex.Unlock()
		if ch != nil {
			ch <- m.cdpReply
		}
	}
}

// cdpCall sends method with params and decodes the result into result,
// which may be nil.
func (c *cdpConnection) cdpCall(ctx context.Context, method string, params any, result any) error {
	c.mutex.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan cdpReply, 1)
	c.waiting[id] = ch
	c.mutex.Unlock()
	b, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		return err
	}
	select {
	case r := <-ch:
		if r.Error != nil {
			return fmt.Errorf("%s: %s", method, r.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(r.Result, result)
		}
		return nil
	case <-c.closed:
		return fmt.Errorf("%s: connection closed: %v", method, c.readErr)
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// cdpEvaluate runs a JavaScript expression in the page, awaits a promise it
// returns and decodes the value, which must be JSON, into result.
func (c *cdpConnection) cdpEvaluate(ctx context.Context, expression string, result any) error {
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	err := c.cdpCall(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "awaitPromise": true, "returnByValue": true,
	}, &r)
	if err != nil {
		return err
	}
	if r.ExceptionDetails != nil {
		return errors.New("javascript: " + r.ExceptionDetails.Text + " " + r.ExceptionDetails.Exception.Description)
	}
	if result != nil && len(r.Result.Value) > 0 {
		return json.Unmarshal(r.Result.Value, result)
	}
	return nil
}

func (c *cdpConnection) cdpClose() {
	c.conn.Close(websocket.StatusNormalClosure, "")
}

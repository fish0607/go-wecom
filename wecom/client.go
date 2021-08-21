package wecom

import (
	"context"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/wenerme/go-req"
)

const DefaultAPI = "https://qyapi.weixin.qq.com"

type Client struct {
	Conf             Conf
	Request          req.Request
	AccessTokenCache TokenResponse
	JsAPITicketCache TicketResponse
	AgentTicketCache TicketResponse
	OnTokenUpdate    func(c *Client)

	updatingToken       int32
	updatingTicket      int32
	updatingAgentTicket int32
}

func WithAccessToken(r *req.Request) (err error) {
	if r.Context == nil {
		return nil
	}
	client := FromContext(r.Context)
	if client != nil {
		var token string
		token, err = client.AccessToken()
		var v url.Values
		if err == nil {
			v, err = req.ValuesOf(r.Query)
			if err == nil {
				if v == nil {
					v = url.Values{}
					r.Query = v
				}
				v.Set("access_token", token)
			}
		}
	}
	return
}

func NewClient(conf Conf) *Client {
	ctx := context.Background()
	c := &Client{
		Conf: conf,
		Request: req.Request{
			BaseURL: DefaultAPI,
			Method:  http.MethodGet,
			Options: []interface{}{req.JSONEncode, req.JSONDecode},
		},
	}
	c.Request.Context = NewContext(ctx, c)
	return c
}

type ClientCache struct {
	AccessToken *TokenResponse
	JsAPITicket *TicketResponse
	AgentTicket *TicketResponse
}

func (c *Client) DumpCache() ClientCache {
	a := c.AccessTokenCache
	b := c.JsAPITicketCache
	d := c.AgentTicketCache
	return ClientCache{
		AccessToken: &a,
		JsAPITicket: &b,
		AgentTicket: &d,
	}
}

func (c *Client) LoadCache(cc *ClientCache) {
	if cc == nil {
		return
	}
	if cc.AccessToken != nil {
		c.AccessTokenCache = *cc.AccessToken
	}
	if cc.JsAPITicket != nil {
		c.JsAPITicketCache = *cc.JsAPITicket
	}
	if cc.AgentTicket != nil {
		c.AgentTicketCache = *cc.AgentTicket
	}
}

func (c *Client) shouldRefresh(ts int64) bool {
	// 默认 2 小时有效期
	// 提前 30 分钟换取
	return ts-time.Now().Unix() < 30*60
}

func (c *Client) JsAPITicket() (string, error) {
	var lastErr error
	if c.shouldRefresh(c.JsAPITicketCache.ExpireAt) {
		if atomic.CompareAndSwapInt32(&c.updatingTicket, 0, 1) {
			defer func() {
				atomic.CompareAndSwapInt32(&c.updatingTicket, 1, 0)
			}()
			var next TicketResponse
			next, lastErr = c.GetJsAPITicket()
			if lastErr != nil {
				logrus.WithError(lastErr).Error("get js api ticket failed")
			} else {
				c.JsAPITicketCache = next
				if c.OnTokenUpdate != nil {
					c.OnTokenUpdate(c)
				}
			}
		}
	}

	cache := c.JsAPITicketCache
	if cache.ExpireAt < time.Now().Unix() {
		if lastErr == nil {
			return "", errors.New("get js api ticket expired or invalid")
		}
		return "", errors.Wrap(lastErr, "get js api ticket failed")
	}
	return cache.Ticket, nil
}

func (c *Client) AgentTicket() (string, error) {
	var lastErr error
	if c.shouldRefresh(c.AgentTicketCache.ExpireAt) {
		if atomic.CompareAndSwapInt32(&c.updatingAgentTicket, 0, 1) {
			defer func() {
				atomic.CompareAndSwapInt32(&c.updatingAgentTicket, 1, 0)
			}()
			var next TicketResponse
			next, lastErr = c.GetAgentTicket()
			if lastErr != nil {
				logrus.WithError(lastErr).Error("get js api ticket failed")
			} else {
				c.AgentTicketCache = next
				if c.OnTokenUpdate != nil {
					c.OnTokenUpdate(c)
				}
			}
		}
	}

	cache := c.AgentTicketCache
	if cache.ExpireAt < time.Now().Unix() {
		if lastErr == nil {
			return "", errors.New("get js api ticket expired or invalid")
		}
		return "", errors.Wrap(lastErr, "get js api ticket failed")
	}
	return cache.Ticket, nil
}

func (c *Client) AccessToken() (string, error) {
	var lastErr error
	if c.shouldRefresh(c.AccessTokenCache.ExpireAt) {
		if atomic.CompareAndSwapInt32(&c.updatingToken, 0, 1) {
			defer func() {
				atomic.CompareAndSwapInt32(&c.updatingToken, 1, 0)
			}()
			var next TokenResponse
			next, lastErr = c.GetToken()
			if lastErr != nil {
				logrus.WithError(lastErr).Error("get js api ticket failed")
			} else {
				c.AccessTokenCache = next
				if c.OnTokenUpdate != nil {
					c.OnTokenUpdate(c)
				}
			}
		}
	}

	cache := c.AccessTokenCache
	if cache.ExpireAt < time.Now().Unix() {
		if lastErr == nil {
			return "", errors.New("get js api ticket expired or invalid")
		}
		return "", errors.Wrap(lastErr, "get js api ticket failed")
	}
	return cache.AccessToken, nil
}

func (c *Client) GetToken() (out TokenResponse, err error) {
	var er GenericResponse
	err = c.Request.With(req.Request{
		URL: "/cgi-bin/gettoken",
		Query: map[string]interface{}{
			"corpid":     c.Conf.CorpID,
			"corpsecret": c.Conf.CorpSecret,
		},
	}).Fetch(&er, &out)
	if err == nil {
		err = er.AsError()
	}
	if err == nil {
		out.ExpireAt = int64(out.ExpiresIn) + time.Now().Unix()
	}
	return out, err
}

func (c *Client) GetJsAPITicket() (out TicketResponse, err error) {
	var er GenericResponse
	err = c.Request.With(req.Request{
		URL:     "/cgi-bin/get_jsapi_ticket",
		Options: []interface{}{WithAccessToken},
	}).Fetch(&er, &out)
	if err == nil {
		err = er.AsError()
	}
	if err == nil {
		out.ExpireAt = int64(out.ExpiresIn) + time.Now().Unix()
	}
	return out, err
}

func (c *Client) GetAgentTicket() (out TicketResponse, err error) {
	var accessToken string
	accessToken, err = c.AccessToken()
	if err != nil {
		return
	}
	var er GenericResponse
	err = c.Request.With(req.Request{
		URL: "/cgi-bin/ticket/get",
		Query: map[string]interface{}{
			"access_token": accessToken,
			"type":         "agent_config",
		},
	}).Fetch(&er, &out)
	if err == nil {
		err = er.AsError()
	}
	if err == nil {
		out.ExpireAt = int64(out.ExpiresIn) + time.Now().Unix()
	}
	return out, err
}

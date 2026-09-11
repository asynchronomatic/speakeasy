package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var ErrRedirectsDisabled = errors.New("redirects disabled")

func denyRedirect(*http.Request, []*http.Request) error {
	return ErrRedirectsDisabled
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Transport interface {
	Get(path string, v any) error
	Post(path string, v any, r any) error
	Put(path string, v any, r any) error
	Delete(path string) error
	SetToken(token string)
}

var ErrClientRequest = 800

type Client struct {
	Address string
	token   string

	rt   Doer
	opts map[string]string
}

func (c *Client) newRequest(method, location string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, location, body)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}
}

func (c *Client) Do(method, location string, in any, out any) error {
	var err error
	client := c.rt

	location = fmt.Sprintf("%s%s", c.Address, location)

	var bodyIn io.Reader

	var data []byte
	if in != nil {
		data, err = json.Marshal(in)
		if err != nil {
			return err
		}
		bodyIn = bytes.NewBuffer(data)
	}

	req, err := c.newRequest(method, location, bodyIn)
	if err != nil {
		return NewError(ErrClientRequest, err.Error())
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return NewError(resp.StatusCode, resp.Status)
	}

	if out != nil {
		return json.Unmarshal(data, out)
	}

	return nil
}

func (c *Client) Delete(location string) error {
	return c.Do(http.MethodDelete, location, nil, nil)
}

func (c *Client) Get(location string, v any) error {
	return c.Do(http.MethodGet, location, nil, v)
}

func (c *Client) Post(location string, in any, out any) error {
	return c.Do(http.MethodPost, location, in, out)
}

func (c *Client) Put(location string, in any, out any) error {
	return c.Do(http.MethodPut, location, in, out)
}

func (c *Client) SetOpt(key, value string) {
	c.opts[key] = value
}

func (c *Client) GetOpt(key string) string {
	return c.opts[key]
}

func (c *Client) Close() error {
	c.token = ""
	return nil
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) WithDoer(rt Doer) *Client {
	c.rt = rt
	return c
}

func NewClient(address, token string) *Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = true

	c := &Client{
		Address: address,
		token:   token,
		opts:    make(map[string]string),
		rt: &http.Client{
			Timeout:       15 * time.Second,
			Transport:     t,
			CheckRedirect: denyRedirect,
		},
	}

	return c
}

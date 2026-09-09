package testable

import (
	"fmt"
	"net/http"

	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

type ProxyClient struct {
	address string
	client  jsonrpc.Doer
	token   string
}

func (c *ProxyClient) DoRaw(req *http.Request) *http.Response {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	return resp
}

func (c *ProxyClient) Do(method, location string, in any, out any) error {
	jc := jsonrpc.NewClient(fmt.Sprintf("http://%s", c.address), c.token).WithDoer(c.client)
	return jc.Do(method, location, in, out)
}

func (c *ProxyClient) LoginGetToken(user, password string) (string, error) {
	req := struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}{
		User:     user,
		Password: password,
	}

	resp := struct {
		Token string `json:"token"`
	}{}

	jc := jsonrpc.NewClient(fmt.Sprintf("http://%s", c.address), "").WithDoer(c.client)
	err := jc.Post("/api/mesh/login", &req, &resp)
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

func (c *ProxyClient) Login(user, password string) error {
	var err error
	c.token, err = c.LoginGetToken(user, password)
	return err
}

func NewProxyClient(address string, p http.HandlerFunc) *ProxyClient {
	c := &ProxyClient{
		address: address,
		client: &Doer{
			Handler: func(w http.ResponseWriter, r *http.Request) {
				p.ServeHTTP(w, r)
			},
		},
	}
	return c
}

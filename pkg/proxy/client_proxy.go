package proxy

import (
	"fmt"

	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

// Client provides an interface to the proxy server directly
type Client struct {
	address string
	client  jsonrpc.Doer
	token   string
}

func (c *Client) Login(user, password string) error {
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
		return err
	}
	c.token = resp.Token
	return nil
}

func (c *Client) GetMeshMembers() ([]NodeStatus, error) {
	resp := MeshMembersResponse{}

	jc := jsonrpc.NewClient(fmt.Sprintf("http://%s", c.address), c.token).WithDoer(c.client)
	err := jc.Get("/api/mesh/members", &resp)
	return resp.Nodes, err
}

func NewProxyClient(address string, client jsonrpc.Doer) *Client {
	c := &Client{
		address: address,
		client:  client,
	}
	return c
}

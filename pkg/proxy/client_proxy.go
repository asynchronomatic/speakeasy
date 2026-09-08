package proxy

import (
	"fmt"

	"github.com/asynchronomatic/speakeasy/pkg/jsonclient"
)

// Client provides an interface to the proxy server directly
type Client struct {
	address string
	client  jsonclient.Doer
}

func (c *Client) GetMeshMembers() ([]NodeStatus, error) {
	resp := MeshMembersResponse{}

	jc := jsonclient.NewClient(fmt.Sprintf("http://%s", c.address), "").WithDoer(c.client)
	err := jc.Get("/api/mesh/members", &resp)
	return resp.Nodes, err
}

func NewProxyClient(address string, client jsonclient.Doer) *Client {
	c := &Client{
		address: address,
		client:  client,
	}
	return c
}

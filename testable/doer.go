package testable

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"

	"github.com/asynchronomatic/speakeasy/pkg/core"
)

type Doer struct {
	dest    core.PeerNode
	Handler http.HandlerFunc
}

func (td *Doer) Do(req *http.Request) (*http.Response, error) {
	if td.Handler == nil {
		return nil, fmt.Errorf("dial failure")
	}

	rw := NewTestResponseWriter()
	td.Handler(rw, req)

	//fmt.Printf("Request: %s", req.URL.Path)
	//fmt.Printf("Response: %s", string(rw.s.Bytes()))

	buf := bufio.NewReader(bytes.NewReader(rw.s.Bytes()))
	return http.ReadResponse(buf, req)
}

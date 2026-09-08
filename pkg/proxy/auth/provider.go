package auth

import (
	"net/http"
)

const (
	AdminUser  = "admin"
	AdminGroup = "admin"
)

type Properties struct {
	User  string
	Group string
}

type Provider interface {
	DoAuth(w http.ResponseWriter, r *http.Request) (*Properties, int)
}

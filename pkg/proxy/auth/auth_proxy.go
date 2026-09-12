package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/jxskiss/base62"
	"golang.org/x/crypto/bcrypt"

	"github.com/asynchronomatic/speakeasy/pkg/security"
)

var encryptionKey = security.EncryptionKey{}

func init() {
	var err error
	encryptionKey, err = security.GenerateKey()
	if err != nil {
		panic(err)
	}
}

type User struct {
	User     string
	Group    string
	Password []byte
}

type sessionClaims struct {
	User    string
	Group   string
	Expires time.Time
}

type UserAuth struct {
	lock  sync.Mutex
	users map[string]User
}

func (a *UserAuth) DoAuth(w http.ResponseWriter, r *http.Request) (*Properties, int) {
	token := security.GetToken(r)
	if token == "" {
		return nil, http.StatusUnauthorized
	}

	session := sessionClaims{}

	err := security.DecryptObject(token, &session, encryptionKey)
	if err != nil {
		return nil, http.StatusUnauthorized
	}

	if session.Expires.Before(time.Now()) {
		return nil, http.StatusUnauthorized
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	user, ok := a.users[session.User]
	if !ok {
		return nil, http.StatusUnauthorized
	}

	return &Properties{
		User:  user.User,
		Group: user.Group,
	}, http.StatusOK
}

func (a *UserAuth) LoginApi(userid, password string) (string, int) {
	a.lock.Lock()
	user, ok := a.users[userid]
	a.lock.Unlock()
	if !ok {
		security.DummySecretMatch(password)
		return "", http.StatusUnauthorized
	}

	err := bcrypt.CompareHashAndPassword(user.Password, []byte(password))
	if err != nil {
		return "", http.StatusUnauthorized
	}

	session := sessionClaims{
		User:    user.User,
		Group:   user.Group,
		Expires: time.Now().Add(time.Hour * 24),
	}

	token, err := security.EncryptObject(&session, encryptionKey)
	if err != nil {
		return "", http.StatusInternalServerError
	}

	return token, http.StatusOK
}

// WithUser adds a new user to the static authenticator
func (a *UserAuth) WithUser(user, group, password string) *UserAuth {
	hashed, err := base62.DecodeString(password)
	if err != nil {
		return nil
	}

	u := User{
		User:     user,
		Group:    group,
		Password: hashed,
	}
	a.users[u.User] = u
	return a
}

func NewUserAuth() *UserAuth {
	return &UserAuth{
		users: make(map[string]User),
	}
}

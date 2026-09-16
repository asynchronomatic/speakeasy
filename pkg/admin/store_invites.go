package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
)

type inviteSecret struct {
	UUID     string
	MeshId   string
	Reusable bool
	Expires  int64
	InviteAs string
}

type InviteStore struct {
	kv     *jsonkv.Store
	meshId string
}

// workflow:
//
//	create an invite link
//	invite link is sent to a user
//	user passes invite link to ./mesh join
//	./mesh join does http.get(link)
//	receives the node id, the server config, and a private token for logins
//	writes confi
func newInviteID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func inviteKVKey(meshID, inviteID string) string {
	return "/invites/" + meshID + "/" + inviteID
}

func invitePublicID(uuid string) string {
	sum := sha256.Sum256([]byte(uuid))
	return hex.EncodeToString(sum[:])
}

func (store *InviteStore) inviteKey(inviteID string) string {
	return inviteKVKey(store.meshId, inviteID)
}

func (store *InviteStore) get(inviteID string) (*inviteSecret, error) {
	var invite inviteSecret
	if err := store.kv.Get(store.inviteKey(inviteID), &invite); err != nil {
		return nil, err

	}
	return &invite, nil
}

func (store *InviteStore) CreateInvite(from string, reusable bool, lifetimeSec uint64) (string, int64, error) {
	inviteUUID, err := newInviteID()
	if err != nil {
		return "", 0, err
	}

	invite := inviteSecret{
		UUID:     inviteUUID,
		Reusable: reusable,
		MeshId:   store.meshId,
		InviteAs: from,
	}

	if lifetimeSec != 0 {
		invite.Expires = time.Now().Add(time.Duration(lifetimeSec) * time.Second).Unix()
	}

	inviteID := invitePublicID(inviteUUID)
	// Invite codes are stored under /invites/<meshID>/<code>.
	// Defaults: one-time, 24h expiry. Forever/reusable require explicit flags.
	if err := store.kv.Put(store.inviteKey(inviteID), invite); err != nil {
		return "", 0, err
	}

	return inviteID, invite.Expires, nil
}

func (store *InviteStore) Redeem(inviteID string, redeem func(*inviteSecret) error) error {
	key := store.inviteKey(inviteID)

	var invite inviteSecret
	if err := store.kv.Get(key, &invite); err != nil {
		if errors.Is(err, jsonkv.ErrNotFound) {
			return jsonrpc.NewError(http.StatusNotFound, "invite not found")
		}
		return err
	}

	if invite.Expires != 0 && time.Now().Unix() >= invite.Expires {
		// do we really want toy delete this
		_ = store.kv.Delete(key)
		return jsonrpc.NewError(http.StatusGone, "invite expired")
	}

	err := redeem(&invite)
	if err != nil {
		return err
	}

	if !invite.Reusable {
		if err := store.kv.Delete(key); err != nil {
			return err
		}
	}

	return nil
}

func (store *InviteStore) ListInvites(base string) ([]api.InviteInfo, error) {
	prefix := fmt.Sprintf("/invites/%s/", store.meshId)

	now := time.Now().Unix()

	invites := make([]api.InviteInfo, 0)
	var expired []string

	err := store.kv.ForEach(prefix, func(key string, data []byte) error {
		var invite inviteSecret
		if err := json.Unmarshal(data, &invite); err != nil {
			return err
		}
		id := strings.TrimPrefix(key, prefix)
		if id == "" {
			return nil
		}
		if invite.Expires != 0 && now >= invite.Expires {
			expired = append(expired, key)
			return nil
		}
		invites = append(invites, api.InviteInfo{
			InviteId:   id,
			InviteLink: fmt.Sprintf("%s/api/v1/redeem/%s", base, id),
			Name:       invite.InviteAs,
			Reusable:   invite.Reusable,
			Expires:    invite.Expires,
			MeshId:     invite.MeshId,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, key := range expired {
		_ = store.kv.Delete(key)
	}
	return invites, nil
}

func (store *InviteStore) Delete(inviteID string) error {
	key := store.inviteKey(inviteID)

	var invite inviteSecret
	if err := store.kv.Get(key, &invite); err != nil {
		if errors.Is(err, jsonkv.ErrNotFound) {
			return jsonrpc.NewError(http.StatusNotFound, "invite not found")
		}
		return err
	}

	if err := store.kv.Delete(key); err != nil {
		return err
	}
	return nil
}

func NewInviteStore(mesh string, kv *jsonkv.Store) *InviteStore {
	return &InviteStore{
		meshId: mesh,
		kv:     kv,
	}
}

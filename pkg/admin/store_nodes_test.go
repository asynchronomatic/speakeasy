package admin

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/jsonkv"
)

func TestNodeStore(t *testing.T) {
	dir := t.TempDir()

	kv, err := jsonkv.Open(path.Join(dir, "admin.jkv"))
	assert.NoError(t, err)
	require.NotNil(t, kv)

	store, err := NewMeshNodeStore("default", kv, NewAllowList())
	assert.NoError(t, err)
	require.NotNil(t, store)

	err = store.NodeLogin("someNode", "someSecret")
	assert.Error(t, err)

	someNode := core.NewPeerNode("someNode", "someName")
	err = store.AddNode(someNode, "test", "someSecret")
	assert.NoError(t, err)

	list := store.NodeList()

	assert.Len(t, list, 1)
	assert.Equal(t, someNode.ID, list[0].ID)

	err = store.NodeLogin("someNode", "someSecret")
	assert.NoError(t, err)

	anotherNode := core.NewPeerNode("anotherNode", "anotherName")
	err = store.AddNode(anotherNode, "test", "anotherSecret")
	assert.NoError(t, err)

	list = store.NodeList()

	assert.Len(t, list, 2)

	anotherInst, anotherLT, err := store.Register(anotherNode.ID)
	assert.NoError(t, err)
	assert.NotEqual(t, "", anotherInst)
	// Out LR should have rolled to 3 at this point Add, Add, Register
	assert.Equal(t, uint64(3), anotherLT)

	err = kv.Close()
	assert.NoError(t, err)

	kv, err = jsonkv.Open(path.Join(dir, "admin.jkv"))
	assert.NoError(t, err)
	require.NotNil(t, kv)

	store, err = NewMeshNodeStore("default", kv, NewAllowList())
	assert.NoError(t, err)
	require.NotNil(t, store)

	list = store.NodeList()
	assert.Len(t, list, 2)

	err = store.NodeLogin("someNode", "someSecret")
	assert.NoError(t, err)

	err = store.DeleteNode(someNode.ID)
	assert.NoError(t, err)

	list = store.NodeList()
	assert.NoError(t, err)
	assert.Len(t, list, 1)

	err = store.NodeLogin("someNode", "someSecret")
	assert.Error(t, err)
}

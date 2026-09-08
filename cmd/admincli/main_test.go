package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	cmd := newCommand()
	var buf bytes.Buffer
	cmd.Writer = &buf
	if err := cmd.Run(context.Background(), []string{"admincli", "--help"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{"Speakeasy", "node", "admin", "redeem", "meshes", "SPEAKEASY_ADMIN_TOKEN"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("help missing %q:\n%s", needle, out)
		}
	}
	if strings.Contains(out, "ModelMesh") || strings.Contains(out, "MODELMESH_") {
		t.Fatalf("help still has old branding:\n%s", out)
	}
}

func TestNodeHelp(t *testing.T) {
	cmd := newCommand()
	var buf bytes.Buffer
	cmd.Writer = &buf
	if err := cmd.Run(context.Background(), []string{"admincli", "node", "--help"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{"list", "register", "unregister", "relay"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("node help missing %q:\n%s", needle, out)
		}
	}
}

func TestAdminHelp(t *testing.T) {
	cmd := newCommand()
	var buf bytes.Buffer
	cmd.Writer = &buf
	if err := cmd.Run(context.Background(), []string{"admincli", "admin", "--help"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{"invite", "delete-invite", "kick"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("admin help missing %q:\n%s", needle, out)
		}
	}
}

func TestRequireToken(t *testing.T) {
	cmd := newCommand()
	cmd.Writer = io.Discard
	cmd.ErrWriter = io.Discard
	err := cmd.Run(context.Background(), []string{"admincli", "node", "list"})
	if err == nil {
		t.Fatal("expected --token required")
	}
}

func TestMeshes(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	cmd := newCommand()
	runErr := cmd.Run(context.Background(), []string{"admincli", "--token", "x", "meshes"})
	_ = w.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatal(runErr)
	}
	buf, _ := io.ReadAll(r)
	if !strings.Contains(string(buf), "default") {
		t.Fatalf("meshes output: %s", buf)
	}
}

package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	cmd := newCommand()
	var buf bytes.Buffer
	cmd.Writer = &buf
	if err := cmd.Run(context.Background(), []string{"mesh", "--help"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{"Speakeasy", "init", "join", "proxy", "admin", "hybrid", "proxy+admin", "reset"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("help missing %q:\n%s", needle, out)
		}
	}
}

func TestJoinHelp(t *testing.T) {
	cmd := newCommand()
	var buf bytes.Buffer
	cmd.Writer = &buf
	if err := cmd.Run(context.Background(), []string{"mesh", "join", "--help"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{"INVITE_URL", "invite"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("join help missing %q:\n%s", needle, out)
		}
	}
}

func TestJoinRequiresURL(t *testing.T) {
	cmd := newCommand()
	cmd.Writer = io.Discard
	cmd.ErrWriter = io.Discard
	err := cmd.Run(context.Background(), []string{"mesh", "join"})
	if err == nil {
		t.Fatal("expected invite URL required")
	}
	if !strings.Contains(err.Error(), "invite URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResetHelp(t *testing.T) {
	cmd := newCommand()
	var buf bytes.Buffer
	cmd.Writer = &buf
	if err := cmd.Run(context.Background(), []string{"mesh", "reset", "--help"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, needle := range []string{"--all", "config.yaml", "node.key"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("reset help missing %q:\n%s", needle, out)
		}
	}
}

func TestHybridAliases(t *testing.T) {
	cmd := newCommand()
	hybrid := cmd.Command("hybrid")
	if hybrid == nil {
		t.Fatal("missing hybrid command")
	}
	for _, alias := range []string{"standalone", "proxy+admin"} {
		if cmd.Command(alias) == nil {
			t.Fatalf("missing hybrid alias %q", alias)
		}
	}
}

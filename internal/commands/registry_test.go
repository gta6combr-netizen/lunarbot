package commands

import (
	"testing"

	"nyxbot-go/internal/config"
)

func TestRegistryAndPermissions(t *testing.T) {
	reg := NewRegistry()
	RegisterAll(reg)

	if len(reg.AllUnique()) < 80 {
		t.Fatalf("expected >=80 commands, got %d", len(reg.AllUnique()))
	}

	if reg.Get("menu") == nil || reg.Get("ajuda") == nil {
		t.Fatal("expected menu and alias ajuda to resolve")
	}
	if reg.Get("s") == nil || reg.Get("sticker") == nil {
		t.Fatal("expected sticker and alias s to resolve")
	}

	cfg := &config.Config{CooldownPadrao: 3}
	ctx := &Context{Config: cfg, IsGroup: true, IsDono: false, IsAdmin: false}
	if ok, _ := reg.CanRun(reg.Get("ban"), ctx); ok {
		t.Fatal("non-admin should not run ban")
	}
	ctx.IsAdmin = true
	ctx.IsBotAdmin = true
	if ok, reason := reg.CanRun(reg.Get("ban"), ctx); !ok {
		t.Fatalf("admin should run ban, got: %s", reason)
	}
}

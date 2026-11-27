package db

import (
	"testing"
)

func TestDBLifecycle(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	jid := "5522999999999@s.whatsapp.net"
	u := d.GetUser(jid)
	if u.Level != 1 || u.Coins != 0 {
		t.Fatalf("unexpected initial user: %+v", u)
	}

	leveled, lvl := d.AddXP(jid, 150, 100, 1.35)
	if !leveled || lvl != 2 {
		t.Fatalf("expected level up to 2, got leveled=%v lvl=%d", leveled, lvl)
	}

	coins := d.AddCoins(jid, 250)
	if coins != 250 {
		t.Fatalf("expected 250 coins, got %d", coins)
	}

	// Group features & mutes & warns
	gid := "120363000000000000@g.us"
	_ = d.SetGroupFeature(gid, "antilink", true)
	if !d.GetGroupFeature(gid, "antilink") {
		t.Fatal("expected antilink to be true")
	}

	_ = d.SetMute(gid, jid, 0)
	if !d.IsMuted(gid, jid) {
		t.Fatal("expected user to be muted")
	}
	_ = d.RemoveMute(gid, jid)
	if d.IsMuted(gid, jid) {
		t.Fatal("expected user to be unmuted")
	}

	if c := d.AddWarn(gid, jid, "spam"); c != 1 {
		t.Fatalf("expected warn count 1, got %d", c)
	}
	if c := d.RemoveWarn(gid, jid); c != 0 {
		t.Fatalf("expected warn count 0, got %d", c)
	}

	// Lunar history & long-term memory
	d.AppendLunarHistory(gid, "user", "[Leo]: oi", 3)
	d.AppendLunarHistory(gid, "model", "oi leo", 3)
	d.AppendLunarHistory(gid, "user", "[Leo]: tudo bem?", 3)
	d.AppendLunarHistory(gid, "model", "tudo ótimo", 3)
	hist := d.GetLunarHistory(gid, 10)
	if len(hist) != 3 || hist[0].Content != "oi leo" || hist[2].Content != "tudo ótimo" {
		t.Fatalf("unexpected lunar history: %+v", hist)
	}

	_ = d.AddLunarMemory("user:"+jid, "gosta de rock")
	_ = d.AddLunarMemory("user:"+jid, "mora no RJ")
	mems := d.GetLunarMemories("user:"+jid, 10)
	if len(mems) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(mems))
	}
	if n, err := d.DeleteLunarMemory("user:"+jid, "rock"); err != nil || n != 1 {
		t.Fatalf("expected 1 deleted memory, got %d (err=%v)", n, err)
	}
	if got := d.rebind("SELECT * FROM x WHERE a = ? AND b = ?"); got != "SELECT * FROM x WHERE a = ? AND b = ?" {
		t.Fatalf("sqlite rebind changed query: %s", got)
	}
	d.isPostgres = true
	if got := d.rebind("SELECT * FROM x WHERE a = ? AND b = ?"); got != "SELECT * FROM x WHERE a = $1 AND b = $2" {
		t.Fatalf("postgres rebind failed: %s", got)
	}
}

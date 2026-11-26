package bot

import (
	"testing"

	"nyxbot-go/internal/commands"
)

func TestNaturalCommands(t *testing.T) {
	reg := commands.NewRegistry()
	commands.RegisterAll(reg)
	c := &Client{registry: reg}

	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{"lunar faz uma figurinha com essa foto", "sticker", true},
		{"lunar faz figurinha com audio", "figaudio", true},
		{"lu0ar manda o menu", "menu", true},
		{"lunar abre o grupo", "abrir", true},
		{"lunar fecha o grupo", "fechar", true},
		{"lunar toca matue", "play", true},
		{"lunar ping", "ping", true},
		{"lunar oi tudo bem?", "", false},
	}

	for _, tc := range cases {
		cmd, _, _, ok := c.parseNaturalCommand(tc.input)
		if ok != tc.ok || cmd != tc.want {
			t.Errorf("parseNaturalCommand(%q) = (%q, %v), want (%q, %v)", tc.input, cmd, ok, tc.want, tc.ok)
		}
	}
}

func TestLunarMentionRegex(t *testing.T) {
	shouldMatch := []string{
		"oi lunar",
		"eai lu0ar tudo bem?",
		"LUNAR BOT vem ca",
		"fala lun4r",
		"lvnar me responde",
		"lunarbot",
	}
	for _, s := range shouldMatch {
		if !lunarMentionRE.MatchString(s) {
			t.Errorf("expected %q to match lunarMentionRE", s)
		}
	}

	shouldNotMatch := []string{
		"olha meu celular",
		"a lua ta linda",
		"bom dia grupo",
	}
	for _, s := range shouldNotMatch {
		if lunarMentionRE.MatchString(s) {
			t.Errorf("did not expect %q to match lunarMentionRE", s)
		}
	}
}

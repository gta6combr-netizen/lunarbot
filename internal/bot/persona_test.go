package bot

import "testing"

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

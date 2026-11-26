package commands

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type velhaGame struct {
	Board   [9]string
	Player1 string
	Player2 string
	Turn    string
}

var (
	gameMu      sync.Mutex
	adivinhaAns = make(map[string]int)
	mathAns     = make(map[string]int)
	anagramAns  = make(map[string]string)
	forcaState  = make(map[string]struct {
		Word    string
		Guessed map[rune]bool
		Errors  int
	})
	velhaGames = make(map[string]*velhaGame)
)

func registerJogos(r *Registry) {
	r.Register(&Command{
		Name: "dado", Category: "jogos", Description: "Rola um dado D6",
		Execute: func(ctx *Context) error {
			return ctx.Reply(fmt.Sprintf("🎲 Você rolou: *%d*", randInt(1, 6)))
		},
	})

	r.Register(&Command{
		Name: "dado20", Aliases: []string{"d20"}, Category: "jogos", Description: "Rola um dado D20",
		Execute: func(ctx *Context) error {
			n := randInt(1, 20)
			extra := ""
			if n == 20 {
				extra = " 🔥 *CRÍTICO!*"
			} else if n == 1 {
				extra = " 💀 *FALHA CRÍTICA!*"
			}
			return ctx.Reply(fmt.Sprintf("🎲 D20: *%d*%s", n, extra))
		},
	})

	r.Register(&Command{
		Name: "moeda", Aliases: []string{"caracoroa"}, Category: "jogos", Description: "Joga cara ou coroa",
		Execute: func(ctx *Context) error {
			res := "🪙 *Cara!*"
			if randInt(0, 1) == 1 {
				res = "🪙 *Coroa!*"
			}
			return ctx.Reply(res)
		},
	})

	r.Register(&Command{
		Name: "slot", Aliases: []string{"cassino"}, Category: "jogos", Description: "Caça-níquel com aposta",
		Execute: func(ctx *Context) error {
			bet := int64(20)
			if len(ctx.Args) > 0 {
				if v, err := strconv.ParseInt(ctx.Args[0], 10, 64); err == nil && v > 0 {
					bet = v
				}
			}
			u := ctx.DB.GetUser(ctx.Sender.String())
			if u.Coins < bet {
				return ctx.Reply(fmt.Sprintf("❌ Saldo insuficiente! Você tem *%d*.", u.Coins))
			}
			syms := []string{"🍒", "🍋", "🍇", "💎", "7️⃣", "🔔"}
			a, b, c := syms[randInt(0, 5)], syms[randInt(0, 5)], syms[randInt(0, 5)]
			var delta int64
			var msg string
			if a == b && b == c {
				delta = bet * 4
				u.Wins++
				msg = fmt.Sprintf("🎰 *JACKPOT!* Ganhou *+%d %s*!", delta, ctx.Config.Moeda.Simbolo)
			} else if a == b || b == c || a == c {
				delta = bet
				u.Wins++
				msg = fmt.Sprintf("🎰 *Dupla!* Ganhou *+%d %s*!", delta, ctx.Config.Moeda.Simbolo)
			} else {
				delta = -bet
				u.Losses++
				msg = fmt.Sprintf("🎰 *Perdeu!* -%d %s", bet, ctx.Config.Moeda.Simbolo)
			}
			u.Coins += delta
			if u.Coins < 0 {
				u.Coins = 0
			}
			_ = ctx.DB.SaveUser(u)
			return ctx.Reply(fmt.Sprintf("[ %s | %s | %s ]\n\n%s\n💰 Saldo: *%d*", a, b, c, msg, u.Coins))
		},
	})

	r.Register(&Command{
		Name: "adivinha", Category: "jogos", Description: "Adivinhe o número de 1 a 100",
		Execute: func(ctx *Context) error {
			chat := ctx.From.String()
			gameMu.Lock()
			target, exists := adivinhaAns[chat]
			if !exists {
				target = randInt(1, 100)
				adivinhaAns[chat] = target
			}
			gameMu.Unlock()

			if ctx.Q == "" {
				return ctx.Reply("🔢 Jogo iniciado! Chute um número de *1 a 100* com `.adivinha <n>`")
			}
			guess, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return ctx.Reply("❗ Informe um número.")
			}
			if guess == target {
				gameMu.Lock()
				delete(adivinhaAns, chat)
				gameMu.Unlock()
				ctx.DB.AddCoins(ctx.Sender.String(), 50)
				return ctx.Reply(fmt.Sprintf("🎉 *ACERTOU!* Era *%d*! Você ganhou *+50 %s*!", target, ctx.Config.Moeda.Simbolo))
			}
			if guess < target {
				return ctx.Reply("⬆️ Mais alto!")
			}
			return ctx.Reply("⬇️ Mais baixo!")
		},
	})

	r.Register(&Command{
		Name: "mathquiz", Category: "jogos", Description: "Desafio rápido de matemática",
		Execute: func(ctx *Context) error {
			chat := ctx.From.String()
			gameMu.Lock()
			ans, ok := mathAns[chat]
			if !ok || ctx.Q == "" {
				a, b := randInt(10, 99), randInt(10, 99)
				mathAns[chat] = a + b
				gameMu.Unlock()
				return ctx.Reply(fmt.Sprintf("🧮 Quanto é *%d + %d*? Responda com `.mathquiz <resposta>`", a, b))
			}
			gameMu.Unlock()
			guess, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return ctx.Reply("❗ Informe um número.")
			}
			if guess == ans {
				gameMu.Lock()
				delete(mathAns, chat)
				gameMu.Unlock()
				ctx.DB.AddCoins(ctx.Sender.String(), 30)
				return ctx.Reply(fmt.Sprintf("✅ Correto! *+30 %s*", ctx.Config.Moeda.Simbolo))
			}
			return ctx.Reply("❌ Errado! Tente novamente.")
		},
	})

	r.Register(&Command{
		Name: "anagrama", Category: "jogos", Description: "Desembaralhe a palavra",
		Execute: func(ctx *Context) error {
			chat := ctx.From.String()
			words := []string{"WHATSAPP", "PROGRAMACAO", "GOLANG", "SERVIDOR", "TERMINAL", "ALGORITMO", "SEGURANCA"}
			gameMu.Lock()
			ans, ok := anagramAns[chat]
			if !ok || ctx.Q == "" {
				w := words[randInt(0, len(words)-1)]
				anagramAns[chat] = w
				runes := []rune(w)
				for i := len(runes) - 1; i > 0; i-- {
					j := randInt(0, i)
					runes[i], runes[j] = runes[j], runes[i]
				}
				gameMu.Unlock()
				return ctx.Reply(fmt.Sprintf("🔤 Desembaralhe: *%s*\nResponda com `.anagrama <palavra>`", string(runes)))
			}
			gameMu.Unlock()
			if strings.EqualFold(ctx.Q, ans) {
				gameMu.Lock()
				delete(anagramAns, chat)
				gameMu.Unlock()
				ctx.DB.AddCoins(ctx.Sender.String(), 40)
				return ctx.Reply(fmt.Sprintf("🎉 Acertou! Era *%s* (+40 %s)", ans, ctx.Config.Moeda.Simbolo))
			}
			return ctx.Reply("❌ Não é essa palavra!")
		},
	})

	r.Register(&Command{
		Name: "forca", Category: "jogos", Description: "Jogo da forca (.forca [letra])",
		Execute: func(ctx *Context) error {
			chat := ctx.From.String()
			words := []string{"ELEFANTE", "COMPUTADOR", "BRASIL", "INTERNET", "RELOGIO", "MONTANHA"}
			gameMu.Lock()
			defer gameMu.Unlock()
			st, ok := forcaState[chat]
			if !ok || ctx.Q == "" {
				st.Word = words[randInt(0, len(words)-1)]
				st.Guessed = make(map[rune]bool)
				st.Errors = 0
				forcaState[chat] = st
				return ctx.Reply("🪢 *Jogo da Forca iniciado!*\n" + renderForca(st.Word, st.Guessed, st.Errors))
			}
			ch := []rune(strings.ToUpper(ctx.Args[0]))[0]
			if !st.Guessed[ch] {
				st.Guessed[ch] = true
				if !strings.ContainsRune(st.Word, ch) {
					st.Errors++
				}
			}
			forcaState[chat] = st
			disp := renderForca(st.Word, st.Guessed, st.Errors)
			if !strings.Contains(disp, "_") {
				delete(forcaState, chat)
				ctx.DB.AddCoins(ctx.Sender.String(), 60)
				return ctx.Reply("🎉 *VOCÊ VENCEU!*\n" + disp + "\n+60 " + ctx.Config.Moeda.Simbolo)
			}
			if st.Errors >= 6 {
				w := st.Word
				delete(forcaState, chat)
				return ctx.Reply("💀 *ENFORCADO!* A palavra era: *" + w + "*")
			}
			return ctx.Reply(disp)
		},
	})

	r.Register(&Command{
		Name: "quiz", Category: "jogos", Description: "Pergunta de conhecimentos gerais",
		Execute: func(ctx *Context) error {
			qs := []struct{ q, a string }{
				{"Qual linguagem foi criada pelo Google em 2009?", "Go"},
				{"Qual o maior planeta do sistema solar?", "Júpiter"},
				{"Em que ano o homem pisou na Lua?", "1969"},
			}
			p := qs[randInt(0, len(qs)-1)]
			return ctx.Reply(fmt.Sprintf("❓ *QUIZ:* %s\n\n||Resposta: %s||", p.q, p.a))
		},
	})

	r.Register(&Command{
		Name: "memoria", Category: "jogos", Description: "Desafio rápido de memória",
		Execute: func(ctx *Context) error {
			seq := fmt.Sprintf("%d-%d-%d-%d", randInt(1, 9), randInt(1, 9), randInt(1, 9), randInt(1, 9))
			return ctx.Reply("🧠 Memorize a sequência: *" + seq + "*")
		},
	})

	r.Register(&Command{
		Name: "rpgstat", Category: "jogos", Description: "Mostra atributos RPG do usuário",
		Execute: func(ctx *Context) error {
			u := ctx.DB.GetUser(ctx.Sender.String())
			atk := 10 + u.Level*3
			def := 8 + u.Level*2
			hp := 100 + u.Level*20
			return ctx.Reply(fmt.Sprintf("⚔️ *FICHA RPG*\n❤️ HP: *%d*\n🗡️ ATK: *%d*\n🛡️ DEF: *%d*\n⭐ Nível: *%d*", hp, atk, def, u.Level))
		},
	})

	r.Register(&Command{
		Name: "velha", Category: "jogos", OnlyGroup: true, Description: "Jogo da velha (.velha @oponente ou .velha 1-9)",
		Execute: func(ctx *Context) error {
			chat := ctx.From.String()
			gameMu.Lock()
			defer gameMu.Unlock()
			g := velhaGames[chat]
			if g == nil || strings.EqualFold(ctx.Q, "novo") {
				target := resolveTarget(ctx)
				if target.IsEmpty() {
					return ctx.Reply("❗ Marque alguém para jogar: `.velha @oponente`")
				}
				g = &velhaGame{
					Board:   [9]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"},
					Player1: ctx.Sender.String(),
					Player2: target.String(),
					Turn:    ctx.Sender.String(),
				}
				velhaGames[chat] = g
				return ctx.Reply("⭕❌ *Jogo da Velha iniciado!*\n" + renderVelha(g))
			}
			if ctx.Sender.String() != g.Turn {
				return ctx.Reply("⏳ Não é sua vez!")
			}
			pos, err := strconv.Atoi(ctx.Q)
			if err != nil || pos < 1 || pos > 9 {
				return ctx.Reply("❗ Escolha uma posição de 1 a 9.")
			}
			idx := pos - 1
			if g.Board[idx] == "X" || g.Board[idx] == "O" {
				return ctx.Reply("❗ Posição já ocupada.")
			}
			mark := "X"
			if ctx.Sender.String() == g.Player2 {
				mark = "O"
			}
			g.Board[idx] = mark
			if checkVelhaWin(g.Board, mark) {
				board := renderVelha(g)
				delete(velhaGames, chat)
				ctx.DB.AddCoins(ctx.Sender.String(), 50)
				return ctx.Reply("🏆 *VITÓRIA!*\n" + board)
			}
			if g.Turn == g.Player1 {
				g.Turn = g.Player2
			} else {
				g.Turn = g.Player1
			}
			return ctx.Reply(renderVelha(g))
		},
	})
}

func renderForca(word string, guessed map[rune]bool, errs int) string {
	var sb strings.Builder
	for _, r := range word {
		if guessed[r] {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
		sb.WriteRune(' ')
	}
	return fmt.Sprintf("`%s`\n❌ Erros: *%d/6*", strings.TrimSpace(sb.String()), errs)
}

func renderVelha(g *velhaGame) string {
	return fmt.Sprintf("`%s | %s | %s`\n`%s | %s | %s`\n`%s | %s | %s`",
		g.Board[0], g.Board[1], g.Board[2],
		g.Board[3], g.Board[4], g.Board[5],
		g.Board[6], g.Board[7], g.Board[8],
	)
}

func checkVelhaWin(b [9]string, m string) bool {
	wins := [8][3]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {0, 3, 6}, {1, 4, 7}, {2, 5, 8}, {0, 4, 8}, {2, 4, 6}}
	for _, w := range wins {
		if b[w[0]] == m && b[w[1]] == m && b[w[2]] == m {
			return true
		}
	}
	return false
}

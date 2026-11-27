package commands

import (
	"fmt"
	"sort"
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
			n := randInt(1, 6)
			cap := fmt.Sprintf("🎲 Você rolou: *%d*", n)
			img, err := renderGameCard("DADO D6", fmt.Sprintf("Resultado: %d", n), []string{"Lado sorteado de 1 a 6."}, colPurple)
			return sendCard(ctx, img, err, cap)
		},
	})

	r.Register(&Command{
		Name: "dado20", Aliases: []string{"d20"}, Category: "jogos", Description: "Rola um dado D20",
		Execute: func(ctx *Context) error {
			n := randInt(1, 20)
			extra := ""
			accent := colCyan
			if n == 20 {
				extra = " 🔥 *CRÍTICO!*"
				accent = colEmerald
			} else if n == 1 {
				extra = " 💀 *FALHA CRÍTICA!*"
				accent = colRose
			}
			cap := fmt.Sprintf("🎲 D20: *%d*%s", n, extra)
			img, err := renderGameCard("DADO D20", fmt.Sprintf("Resultado: %d", n), []string{strings.TrimSpace(extra)}, accent)
			return sendCard(ctx, img, err, cap)
		},
	})

	r.Register(&Command{
		Name: "moeda", Aliases: []string{"caracoroa"}, Category: "jogos", Description: "Joga cara ou coroa",
		Execute: func(ctx *Context) error {
			side := "Cara!"
			accent := colAmber
			if randInt(0, 1) == 1 {
				side = "Coroa!"
				accent = colCyan
			}
			img, err := renderGameCard("CARA OU COROA", side, []string{"Moeda lançada."}, accent)
			return sendCard(ctx, img, err, "🪙 *"+side+"*")
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
				cap := fmt.Sprintf("❌ Saldo insuficiente! Você tem *%d*.", u.Coins)
				img, err := renderGameCard("CASSINO SLOT", "Saldo insuficiente", []string{fmt.Sprintf("Você tem %d %s (aposta mínima: %d).", u.Coins, ctx.Config.Moeda.Simbolo, bet)}, colRose)
				return sendCard(ctx, img, err, cap)
			}
			syms := []string{"🍒", "🍋", "🍇", "💎", "7️⃣", "🔔"}
			names := []string{"Cereja", "Limão", "Uva", "Diamante", "7", "Sino"}
			ia, ib, ic := randInt(0, 5), randInt(0, 5), randInt(0, 5)
			usedClover := false
			if ia != ib && ib != ic && ia != ic {
				if newInv, ok := consumeInventoryItem(u.Inventory, "Trevo"); ok {
					u.Inventory = newInv
					usedClover = true
					ia, ib, ic = randInt(0, 5), randInt(0, 5), randInt(0, 5)
				}
			}
			a, b, c := syms[ia], syms[ib], syms[ic]
			var delta int64
			var msg string
			accent := colRose
			if a == b && b == c {
				delta = bet * 4
				u.Wins++
				msg = fmt.Sprintf("🎰 *JACKPOT!* Ganhou *+%d %s*!", delta, ctx.Config.Moeda.Simbolo)
				accent = colEmerald
			} else if a == b || b == c || a == c {
				delta = bet
				u.Wins++
				msg = fmt.Sprintf("🎰 *Dupla!* Ganhou *+%d %s*!", delta, ctx.Config.Moeda.Simbolo)
				accent = colAmber
			} else {
				delta = -bet
				u.Losses++
				msg = fmt.Sprintf("🎰 *Perdeu!* -%d %s", bet, ctx.Config.Moeda.Simbolo)
			}
			if usedClover {
				msg = "🍀 *Trevo da Sorte usado!* " + msg
			}
			u.Coins += delta
			if u.Coins < 0 {
				u.Coins = 0
			}
			_ = ctx.DB.SaveUser(u)
			cap := fmt.Sprintf("[ %s | %s | %s ]\n\n%s\n💰 Saldo: *%d*", a, b, c, msg, u.Coins)
			img, err := renderGameCard("CASSINO SLOT", fmt.Sprintf("[ %s | %s | %s ]", names[ia], names[ib], names[ic]), []string{msg, fmt.Sprintf("Saldo atual: %d %s", u.Coins, ctx.Config.Moeda.Simbolo)}, accent)
			return sendCard(ctx, img, err, cap)
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
				cap := "🔢 Jogo iniciado! Chute um número de *1 a 100* com `.adivinha <n>`"
				img, err := renderGameCard("ADIVINHA", "Chute de 1 a 100", []string{"Use .adivinha <numero> para tentar."}, colPurple)
				return sendCard(ctx, img, err, cap)
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
				cap := fmt.Sprintf("🎉 *ACERTOU!* Era *%d*! Você ganhou *+50 %s*!", target, ctx.Config.Moeda.Simbolo)
				img, err := renderGameCard("ADIVINHA", fmt.Sprintf("Acertou! Era %d", target), []string{"Recompensa: +50 " + ctx.Config.Moeda.Simbolo}, colEmerald)
				return sendCard(ctx, img, err, cap)
			}
			if guess < target {
				img, err := renderGameCard("ADIVINHA", fmt.Sprintf("Chute: %d", guess), []string{"Dica: Mais ALTO!"}, colAmber)
				return sendCard(ctx, img, err, "⬆️ Mais alto!")
			}
			img, err := renderGameCard("ADIVINHA", fmt.Sprintf("Chute: %d", guess), []string{"Dica: Mais BAIXO!"}, colCyan)
			return sendCard(ctx, img, err, "⬇️ Mais baixo!")
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
				cap := fmt.Sprintf("🧮 Quanto é *%d + %d*? Responda com `.mathquiz <resposta>`", a, b)
				img, err := renderGameCard("MATH QUIZ", fmt.Sprintf("Quanto é %d + %d ?", a, b), []string{"Responda com .mathquiz <resposta>"}, colCyan)
				return sendCard(ctx, img, err, cap)
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
				cap := fmt.Sprintf("✅ Correto! *+30 %s*", ctx.Config.Moeda.Simbolo)
				img, err := renderGameCard("MATH QUIZ", fmt.Sprintf("Correto! Resposta: %d", ans), []string{"+30 " + ctx.Config.Moeda.Simbolo}, colEmerald)
				return sendCard(ctx, img, err, cap)
			}
			img, err := renderGameCard("MATH QUIZ", "Resposta errada!", []string{"Tente novamente com .mathquiz <numero>"}, colRose)
			return sendCard(ctx, img, err, "❌ Errado! Tente novamente.")
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
				scr := string(runes)
				cap := fmt.Sprintf("🔤 Desembaralhe: *%s*\nResponda com `.anagrama <palavra>`", scr)
				img, err := renderGameCard("ANAGRAMA", scr, []string{"Desembaralhe a palavra e responda com .anagrama <palavra>"}, colPurple)
				return sendCard(ctx, img, err, cap)
			}
			gameMu.Unlock()
			if strings.EqualFold(ctx.Q, ans) {
				gameMu.Lock()
				delete(anagramAns, chat)
				gameMu.Unlock()
				ctx.DB.AddCoins(ctx.Sender.String(), 40)
				cap := fmt.Sprintf("🎉 Acertou! Era *%s* (+40 %s)", ans, ctx.Config.Moeda.Simbolo)
				img, err := renderGameCard("ANAGRAMA", "Acertou: "+ans, []string{"+40 " + ctx.Config.Moeda.Simbolo}, colEmerald)
				return sendCard(ctx, img, err, cap)
			}
			img, err := renderGameCard("ANAGRAMA", "Não é essa palavra!", []string{"Tente novamente."}, colRose)
			return sendCard(ctx, img, err, "❌ Não é essa palavra!")
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
				mask, used := forcaMaskAndUsed(st.Word, st.Guessed)
				img, err := renderForcaCard(mask, st.Errors, 6, used, "Use .forca <letra> para jogar")
				return sendCard(ctx, img, err, "🪢 *Jogo da Forca iniciado!*\n"+renderForca(st.Word, st.Guessed, st.Errors))
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
			mask, used := forcaMaskAndUsed(st.Word, st.Guessed)
			if !strings.Contains(disp, "_") {
				delete(forcaState, chat)
				ctx.DB.AddCoins(ctx.Sender.String(), 60)
				img, err := renderForcaCard(mask, st.Errors, 6, used, "VOCÊ VENCEU! +60 "+ctx.Config.Moeda.Simbolo)
				return sendCard(ctx, img, err, "🎉 *VOCÊ VENCEU!*\n"+disp+"\n+60 "+ctx.Config.Moeda.Simbolo)
			}
			if st.Errors >= 6 {
				w := st.Word
				delete(forcaState, chat)
				img, err := renderForcaCard(w, 6, 6, used, "ENFORCADO! Palavra: "+w)
				return sendCard(ctx, img, err, "💀 *ENFORCADO!* A palavra era: *"+w+"*")
			}
			img, err := renderForcaCard(mask, st.Errors, 6, used, "Continue: .forca <letra>")
			return sendCard(ctx, img, err, disp)
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
			cap := fmt.Sprintf("❓ *QUIZ:* %s\n\n||Resposta: %s||", p.q, p.a)
			img, err := renderGameCard("QUIZ", "Conhecimentos Gerais", []string{p.q, "(Resposta oculta na legenda)"}, colAmber)
			return sendCard(ctx, img, err, cap)
		},
	})

	r.Register(&Command{
		Name: "memoria", Category: "jogos", Description: "Desafio rápido de memória",
		Execute: func(ctx *Context) error {
			seq := fmt.Sprintf("%d - %d - %d - %d", randInt(1, 9), randInt(1, 9), randInt(1, 9), randInt(1, 9))
			img, err := renderGameCard("MEMÓRIA", seq, []string{"Memorize esta sequência!"}, colPurple)
			return sendCard(ctx, img, err, "🧠 Memorize a sequência: *"+seq+"*")
		},
	})

	r.Register(&Command{
		Name: "rpgstat", Category: "jogos", Description: "Mostra atributos RPG do usuário",
		Execute: func(ctx *Context) error {
			u := ctx.DB.GetUser(ctx.Sender.String())
			atk := 10 + u.Level*3
			def := 8 + u.Level*2
			hp := 100 + u.Level*20
			cap := fmt.Sprintf("⚔️ *FICHA RPG*\n❤️ HP: *%d*\n🗡️ ATK: *%d*\n🛡️ DEF: *%d*\n⭐ Nível: *%d*", hp, atk, def, u.Level)
			img, err := renderGameCard("FICHA RPG", fmt.Sprintf("Nível %d", u.Level), []string{
				fmt.Sprintf("HP: %d   |   ATK: %d   |   DEF: %d", hp, atk, def),
				fmt.Sprintf("Vitórias: %d   |   Derrotas: %d", u.Wins, u.Losses),
			}, colEmerald)
			return sendCard(ctx, img, err, cap)
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
				img, err := renderVelhaCard(g.Board, "Vez do X! Use .velha <1-9>", shortJID(g.Player1), shortJID(g.Player2))
				return sendCard(ctx, img, err, "⭕❌ *Jogo da Velha iniciado!*\n"+renderVelha(g))
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
				bCopy := g.Board
				p1, p2 := shortJID(g.Player1), shortJID(g.Player2)
				delete(velhaGames, chat)
				ctx.DB.AddCoins(ctx.Sender.String(), 50)
				img, err := renderVelhaCard(bCopy, fmt.Sprintf("VITÓRIA DE %s! (+50 %s)", mark, ctx.Config.Moeda.Simbolo), p1, p2)
				return sendCard(ctx, img, err, "🏆 *VITÓRIA!*\n"+board)
			}
			if checkVelhaDraw(g.Board) {
				board := renderVelha(g)
				bCopy := g.Board
				p1, p2 := shortJID(g.Player1), shortJID(g.Player2)
				delete(velhaGames, chat)
				img, err := renderVelhaCard(bCopy, "EMPATE! Deu velha!", p1, p2)
				return sendCard(ctx, img, err, "🤝 *EMPATE!* Deu velha!\n"+board)
			}
			nextMark := "O"
			if g.Turn == g.Player1 {
				g.Turn = g.Player2
			} else {
				g.Turn = g.Player1
				nextMark = "X"
			}
			img, err := renderVelhaCard(g.Board, fmt.Sprintf("Vez do %s! Use .velha <1-9>", nextMark), shortJID(g.Player1), shortJID(g.Player2))
			return sendCard(ctx, img, err, renderVelha(g))
		},
	})
}

func shortJID(jid string) string {
	if i := strings.IndexByte(jid, '@'); i > 0 {
		return jid[:i]
	}
	return jid
}

func forcaMaskAndUsed(word string, guessed map[rune]bool) (string, string) {
	var sb strings.Builder
	for _, r := range word {
		if guessed[r] {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
		sb.WriteRune(' ')
	}
	var letters []string
	for r := range guessed {
		letters = append(letters, string(r))
	}
	sort.Strings(letters)
	return strings.TrimSpace(sb.String()), strings.Join(letters, " ")
}

func checkVelhaDraw(b [9]string) bool {
	for _, v := range b {
		if v != "X" && v != "O" {
			return false
		}
	}
	return true
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

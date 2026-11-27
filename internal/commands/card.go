package commands

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	parsedBoldFont    *opentype.Font
	parsedRegularFont *opentype.Font

	colBG       = color.RGBA{13, 15, 26, 255}
	colCard     = color.RGBA{23, 27, 46, 255}
	colCardDark = color.RGBA{17, 20, 35, 255}
	colPurple   = color.RGBA{139, 92, 246, 255}
	colCyan     = color.RGBA{56, 189, 248, 255}
	colRose     = color.RGBA{244, 63, 94, 255}
	colEmerald  = color.RGBA{16, 185, 129, 255}
	colAmber    = color.RGBA{245, 158, 11, 255}
	colText     = color.RGBA{241, 245, 249, 255}
	colMuted    = color.RGBA{148, 163, 184, 255}
	colGrid     = color.RGBA{51, 65, 85, 255}
)

func init() {
	parsedBoldFont, _ = opentype.Parse(gobold.TTF)
	parsedRegularFont, _ = opentype.Parse(goregular.TTF)
}

func makeFace(bold bool, size float64) font.Face {
	f := parsedRegularFont
	if bold {
		f = parsedBoldFont
	}
	if f == nil {
		return nil
	}
	face, _ := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	return face
}

func stripEmojiForFont(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r <= 0x024F || r == '•' || r == '—' || r == '–' || unicode.IsSpace(r) || (r >= 0x2000 && r <= 0x206F) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
}

func fillRoundedRect(img *image.RGBA, x, y, w, h, r int, c color.RGBA) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			dx, dy := 0, 0
			if px < x+r && py < y+r {
				dx, dy = (x+r)-px, (y+r)-py
			} else if px >= x+w-r && py < y+r {
				dx, dy = px-(x+w-r-1), (y+r)-py
			} else if px < x+r && py >= y+h-r {
				dx, dy = (x+r)-px, py-(y+h-r-1)
			} else if px >= x+w-r && py >= y+h-r {
				dx, dy = px-(x+w-r-1), py-(y+h-r-1)
			}
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

func drawThickLine(img *image.RGBA, x0, y0, x1, y1, thickness int, c color.RGBA) {
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	steps := int(math.Max(math.Abs(dx), math.Abs(dy))) + 1
	rad := thickness / 2
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		cx := int(math.Round(float64(x0) + dx*t))
		cy := int(math.Round(float64(y0) + dy*t))
		for oy := -rad; oy <= rad; oy++ {
			for ox := -rad; ox <= rad; ox++ {
				if ox*ox+oy*oy <= rad*rad+1 {
					img.SetRGBA(cx+ox, cy+oy, c)
				}
			}
		}
	}
}

func drawCircleRing(img *image.RGBA, cx, cy, r, thickness int, c color.RGBA) {
	rIn := float64(r - thickness/2)
	rOut := float64(r + thickness/2)
	for py := cy - r - thickness; py <= cy+r+thickness; py++ {
		for px := cx - r - thickness; px <= cx+r+thickness; px++ {
			d := math.Hypot(float64(px-cx), float64(py-cy))
			if d >= rIn && d <= rOut {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

func drawText(img *image.RGBA, face font.Face, x, y int, c color.RGBA, text string) {
	if face == nil {
		return
	}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}

func drawCenteredText(img *image.RGBA, face font.Face, cx, y int, c color.RGBA, text string) {
	if face == nil {
		return
	}
	w := font.MeasureString(face, text).Ceil()
	drawText(img, face, cx-w/2, y, c, text)
}

func wrapText(face font.Face, text string, maxWidth int) []string {
	text = stripEmojiForFont(text)
	if text == "" {
		return nil
	}
	var splitWords []string
	for _, w := range strings.Fields(text) {
		if face == nil || font.MeasureString(face, w).Ceil() <= maxWidth {
			splitWords = append(splitWords, w)
			continue
		}
		var chunk string
		for _, r := range w {
			cand := chunk + string(r)
			if chunk != "" && font.MeasureString(face, cand).Ceil() > maxWidth {
				splitWords = append(splitWords, chunk)
				chunk = string(r)
			} else {
				chunk = cand
			}
		}
		if chunk != "" {
			splitWords = append(splitWords, chunk)
		}
	}
	if len(splitWords) == 0 {
		return nil
	}
	var lines []string
	cur := splitWords[0]
	for _, w := range splitWords[1:] {
		cand := cur + " " + w
		if face != nil && font.MeasureString(face, cand).Ceil() > maxWidth {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = cand
		}
	}
	lines = append(lines, cur)
	return lines
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderGameCard(tag, title string, bodyLines []string, accent color.RGBA) ([]byte, error) {
	fTag := makeFace(true, 18)
	fTitle := makeFace(true, 34)
	fBody := makeFace(false, 25)
	fFoot := makeFace(false, 16)
	defer func() {
		for _, f := range []font.Face{fTag, fTitle, fBody, fFoot} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	var wrapped []string
	for _, raw := range bodyLines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, wrapText(fBody, raw, 760)...)
	}
	if len(wrapped) > 10 {
		wrapped = wrapped[:10]
	}

	w := 880
	h := 240 + len(wrapped)*38
	if h < 320 {
		h = 320
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colBG)
	fillRoundedRect(img, 24, 24, w-48, h-48, 22, accent)
	fillRoundedRect(img, 27, 27, w-54, h-54, 20, colCard)

	// Header pill
	cleanTag := strings.ToUpper(stripEmojiForFont(tag))
	if cleanTag == "" {
		cleanTag = "LUNAR"
	}
	fillRoundedRect(img, 54, 52, 220, 34, 10, accent)
	drawCenteredText(img, fTag, 164, 75, colBG, cleanTag)

	drawText(img, fTitle, 54, 134, colText, stripEmojiForFont(title))

	y := 188
	for _, line := range wrapped {
		if line != "" {
			drawText(img, fBody, 54, y, colMuted, line)
		}
		y += 38
	}

	drawText(img, fFoot, 54, h-46, colGrid, "LUNAR BOT  •  JOGOS & RESENHA")
	return encodePNG(img)
}

func renderVelhaCard(board [9]string, status, xName, oName string) ([]byte, error) {
	w, h := 720, 820
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colBG)
	fillRoundedRect(img, 24, 24, w-48, h-48, 24, colPurple)
	fillRoundedRect(img, 27, 27, w-54, h-54, 22, colCard)

	fTitle := makeFace(true, 32)
	fSub := makeFace(false, 22)
	fNum := makeFace(true, 44)
	fStatus := makeFace(true, 24)
	defer func() {
		for _, f := range []font.Face{fTitle, fSub, fNum, fStatus} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	drawCenteredText(img, fTitle, w/2, 80, colText, "JOGO DA VELHA")
	players := fmt.Sprintf("X: %s   vs   O: %s", stripEmojiForFont(xName), stripEmojiForFont(oName))
	drawCenteredText(img, fSub, w/2, 118, colMuted, players)

	// Board box
	bx, by, size := 120, 160, 480
	cell := size / 3
	fillRoundedRect(img, bx, by, size, size, 18, colCardDark)

	// Grid lines
	drawThickLine(img, bx+cell, by+18, bx+cell, by+size-18, 6, colGrid)
	drawThickLine(img, bx+2*cell, by+18, bx+2*cell, by+size-18, 6, colGrid)
	drawThickLine(img, bx+18, by+cell, bx+size-18, by+cell, 6, colGrid)
	drawThickLine(img, bx+18, by+2*cell, bx+size-18, by+2*cell, 6, colGrid)

	for i := 0; i < 9; i++ {
		r := i / 3
		c := i % 3
		cx := bx + c*cell + cell/2
		cy := by + r*cell + cell/2
		v := board[i]
		switch v {
		case "❌", "X":
			off := 40
			drawThickLine(img, cx-off, cy-off, cx+off, cy+off, 12, colRose)
			drawThickLine(img, cx+off, cy-off, cx-off, cy+off, 12, colRose)
		case "⭕", "O":
			drawCircleRing(img, cx, cy, 42, 12, colCyan)
		default:
			drawCenteredText(img, fNum, cx, cy+16, colGrid, fmt.Sprintf("%d", i+1))
		}
	}

	fillRoundedRect(img, 60, 680, w-120, 74, 14, colCardDark)
	drawCenteredText(img, fStatus, w/2, 725, colText, stripEmojiForFont(status))
	return encodePNG(img)
}

func renderForcaCard(wordMask string, errs, maxErrs int, used, status string) ([]byte, error) {
	w, h := 840, 540
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colBG)
	fillRoundedRect(img, 24, 24, w-48, h-48, 24, colCyan)
	fillRoundedRect(img, 27, 27, w-54, h-54, 22, colCard)

	fTitle := makeFace(true, 30)
	fWord := makeFace(true, 44)
	fMeta := makeFace(false, 22)
	fStatus := makeFace(true, 22)
	defer func() {
		for _, f := range []font.Face{fTitle, fWord, fMeta, fStatus} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	drawText(img, fTitle, 60, 82, colText, "JOGO DA FORCA")

	// Gallows on left
	gx, gy := 90, 125
	drawThickLine(img, gx, gy+260, gx+150, gy+260, 8, colMuted)
	drawThickLine(img, gx+40, gy+260, gx+40, gy, 8, colMuted)
	drawThickLine(img, gx+40, gy, gx+160, gy, 8, colMuted)
	drawThickLine(img, gx+160, gy, gx+160, gy+40, 6, colAmber)

	if errs >= 1 {
		drawCircleRing(img, gx+160, gy+66, 26, 6, colRose)
	}
	if errs >= 2 {
		drawThickLine(img, gx+160, gy+92, gx+160, gy+172, 6, colRose)
	}
	if errs >= 3 {
		drawThickLine(img, gx+160, gy+110, gx+126, gy+145, 6, colRose)
	}
	if errs >= 4 {
		drawThickLine(img, gx+160, gy+110, gx+194, gy+145, 6, colRose)
	}
	if errs >= 5 {
		drawThickLine(img, gx+160, gy+172, gx+132, gy+222, 6, colRose)
	}
	if errs >= 6 {
		drawThickLine(img, gx+160, gy+172, gx+188, gy+222, 6, colRose)
	}

	// Word & info on right
	drawText(img, fWord, 330, 210, colCyan, wordMask)
	drawText(img, fMeta, 330, 275, colMuted, fmt.Sprintf("Erros: %d / %d", errs, maxErrs))
	if used != "" {
		drawText(img, fMeta, 330, 315, colMuted, "Letras: "+used)
	}

	fillRoundedRect(img, 60, 420, w-120, 62, 14, colCardDark)
	drawCenteredText(img, fStatus, w/2, 459, colText, stripEmojiForFont(status))
	return encodePNG(img)
}

func renderMeterCard(title, target string, pct int, subline string, accent color.RGBA) ([]byte, error) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	w, h := 840, 360
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colBG)
	fillRoundedRect(img, 24, 24, w-48, h-48, 22, accent)
	fillRoundedRect(img, 27, 27, w-54, h-54, 20, colCard)

	fTitle := makeFace(true, 30)
	fPct := makeFace(true, 56)
	fSub := makeFace(false, 24)
	defer func() {
		for _, f := range []font.Face{fTitle, fPct, fSub} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	drawText(img, fTitle, 56, 86, colText, stripEmojiForFont(title))
	drawText(img, fSub, 56, 128, colMuted, stripEmojiForFont(target))
	drawText(img, fPct, w-210, 125, accent, fmt.Sprintf("%d%%", pct))

	// Progress bar
	barX, barY, barW, barH := 56, 175, w-112, 34
	fillRoundedRect(img, barX, barY, barW, barH, 14, colCardDark)
	fillW := (barW * pct) / 100
	if fillW < 28 && pct > 0 {
		fillW = 28
	}
	if fillW > 0 {
		fillRoundedRect(img, barX, barY, fillW, barH, 14, accent)
	}

	if subline != "" {
		drawText(img, fSub, 56, 265, colText, stripEmojiForFont(subline))
	}
	return encodePNG(img)
}

type rankEntry struct {
	Pos    int
	Name   string
	Stat   string
	Avatar []byte
}

func drawCircularAvatar(dst *image.RGBA, raw []byte, cx, cy, r int, fallbackText string, fallbackCol color.RGBA) {
	if len(raw) > 0 {
		if src, _, err := image.Decode(bytes.NewReader(raw)); err == nil {
			b := src.Bounds()
			sw, sh := b.Dx(), b.Dy()
			side := sw
			if sh < side {
				side = sh
			}
			ox := b.Min.X + (sw-side)/2
			oy := b.Min.Y + (sh-side)/2
			for py := -r; py <= r; py++ {
				for px := -r; px <= r; px++ {
					if px*px+py*py <= r*r {
						sx := ox + ((px + r) * side / (2 * r))
						sy := oy + ((py + r) * side / (2 * r))
						dst.Set(cx+px, cy+py, src.At(sx, sy))
					}
				}
			}
			return
		}
	}
	for py := -r; py <= r; py++ {
		for px := -r; px <= r; px++ {
			if px*px+py*py <= r*r {
				dst.SetRGBA(cx+px, cy+py, fallbackCol)
			}
		}
	}
	if f := makeFace(true, float64(r)); f != nil {
		defer f.Close()
		drawCenteredText(dst, f, cx, cy+r/3, colBG, fallbackText)
	}
}

func renderRankCard(rootDir, title string, entries []rankEntry, accent color.RGBA) ([]byte, error) {
	fTag := makeFace(true, 16)
	fTitle := makeFace(true, 30)
	fPos := makeFace(true, 22)
	fName := makeFace(true, 21)
	fStat := makeFace(true, 20)
	fFoot := makeFace(false, 15)
	defer func() {
		for _, f := range []font.Face{fTag, fTitle, fPos, fName, fStat, fFoot} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	if len(entries) > 8 {
		entries = entries[:8]
	}

	var img *image.RGBA
	w, h := 1024, 640
	bgPath := filepath.Join(rootDir, "assets", "rank-bg.png")
	if rawBG, err := os.ReadFile(bgPath); err == nil {
		if srcBG, _, err := image.Decode(bytes.NewReader(rawBG)); err == nil {
			b := srcBG.Bounds()
			w, h = b.Dx(), b.Dy()
			img = image.NewRGBA(image.Rect(0, 0, w, h))
			draw.Draw(img, img.Bounds(), srcBG, b.Min, draw.Src)
		}
	}
	if img == nil {
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		fillRect(img, 0, 0, w, h, colBG)
		fillRoundedRect(img, 72, 48, w-144, h-96, 24, colAmber)
		fillRoundedRect(img, 76, 52, w-152, h-104, 22, colCard)
	}

	colGold := color.RGBA{245, 158, 11, 255}
	colGoldBright := color.RGBA{251, 191, 36, 255}
	colRowBorder := color.RGBA{180, 105, 18, 255}
	colRowBG := color.RGBA{18, 15, 22, 255}

	fillRoundedRect(img, 124, 84, 194, 30, 14, colGold)
	drawCenteredText(img, fTag, 221, 105, colBG, "RANKING LUNAR")
	drawText(img, fTitle, 124, 150, colText, stripEmojiForFont(title))

	rowH, gap, avR := 60, 10, 22
	if len(entries) > 5 {
		rowH, gap, avR = 42, 6, 16
	}
	y := 172
	for _, e := range entries {
		fillRoundedRect(img, 124, y, w-248, rowH, 16, colRowBorder)
		fillRoundedRect(img, 126, y+2, w-252, rowH-4, 14, colRowBG)

		posCol := colMuted
		switch e.Pos {
		case 1:
			posCol = colGoldBright
		case 2:
			posCol = color.RGBA{203, 213, 225, 255}
		case 3:
			posCol = color.RGBA{217, 119, 6, 255}
		}
		drawCenteredText(img, fPos, 164, y+rowH/2+8, posCol, fmt.Sprintf("#%d", e.Pos))

		drawCircleRing(img, 222, y+rowH/2, avR+2, 2, posCol)
		drawCircularAvatar(img, e.Avatar, 222, y+rowH/2, avR, fmt.Sprintf("%d", e.Pos), posCol)

		name := stripEmojiForFont(e.Name)
		if len(name) > 22 {
			name = name[:22] + "..."
		}
		drawText(img, fName, 264, y+rowH/2+7, colText, name)

		stat := stripEmojiForFont(e.Stat)
		sw := 0
		if fStat != nil {
			sw = font.MeasureString(fStat, stat).Ceil()
		}
		drawText(img, fStat, w-148-sw, y+rowH/2+7, colGoldBright, stat)
		y += rowH + gap
	}

	drawCenteredText(img, fFoot, w/2, h-78, colMuted, "LUNAR BOT  •  TOP RANKING")
	return encodePNG(img)
}

func drawHeart(img *image.RGBA, cx, cy, size int, c color.RGBA) {
	s := float64(size)
	for py := -size * 2; py <= size*2; py++ {
		for px := -size * 2; px <= size*2; px++ {
			x := float64(px) / s
			y := -float64(py) / s
			v := math.Pow(x*x+y*y-1, 3) - x*x*y*y*y
			if v <= 0 {
				img.SetRGBA(cx+px, cy+py, c)
			}
		}
	}
}

func renderCantadaCard(quote, fromName, toName string, fromAvatar, toAvatar []byte) ([]byte, error) {
	w, h := 900, 480
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// Deep romantic gradient + soft pink glow
	for py := 0; py < h; py++ {
		t := float64(py) / float64(h)
		r := uint8(24 + 22*math.Sin(t*math.Pi))
		g := uint8(10 + 6*math.Sin(t*math.Pi))
		b := uint8(34 + 24*math.Sin(t*math.Pi))
		for px := 0; px < w; px++ {
			dGlow := math.Hypot(float64(px-w/2), float64(py-h/2))
			glow := math.Max(0, 1.0-dGlow/420.0)
			rr := uint8(math.Min(255, float64(r)+glow*46))
			gg := uint8(math.Min(255, float64(g)+glow*10))
			bb := uint8(math.Min(255, float64(b)+glow*34))
			img.SetRGBA(px, py, color.RGBA{rr, gg, bb, 255})
		}
	}

	colPink := color.RGBA{244, 114, 182, 255}
	colHotPink := color.RGBA{236, 72, 153, 255}
	colWarmWhite := color.RGBA{255, 241, 242, 255}
	colSoftPink := color.RGBA{251, 207, 232, 255}

	fillRoundedRect(img, 28, 28, w-56, h-56, 26, colPink)
	fillRoundedRect(img, 31, 31, w-62, h-62, 24, color.RGBA{22, 11, 30, 255})
	fillRoundedRect(img, 44, 44, w-88, h-88, 18, color.RGBA{34, 16, 44, 255})

	fTag := makeFace(true, 18)
	fQuote := makeFace(true, 28)
	fSub := makeFace(false, 20)
	fFoot := makeFace(false, 16)
	defer func() {
		for _, f := range []font.Face{fTag, fQuote, fSub, fFoot} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	// Header pill + vector heart
	fillRoundedRect(img, 68, 66, 220, 34, 12, colHotPink)
	drawCenteredText(img, fTag, 178, 89, colBG, "MODO ROMANCE")
	drawHeart(img, 315, 84, 12, colRose)

	// Avatars on top-right
	if toName != "" {
		drawCircularAvatar(img, fromAvatar, w-196, 86, 24, "EU", colPink)
		drawHeart(img, w-146, 86, 9, colRose)
		drawCircularAvatar(img, toAvatar, w-96, 86, 24, "VC", colHotPink)
	} else if len(fromAvatar) > 0 {
		drawCircularAvatar(img, fromAvatar, w-96, 86, 24, "L", colPink)
	}

	// Centered wrapped quote
	lines := wrapText(fQuote, `"`+quote+`"`, 720)
	if len(lines) > 5 {
		lines = lines[:5]
	}
	startY := 220 - (len(lines)-1)*22
	for _, line := range lines {
		drawCenteredText(img, fQuote, w/2, startY, colWarmWhite, line)
		startY += 44
	}

	if toName != "" {
		drawCenteredText(img, fSub, w/2, h-102, colSoftPink, fmt.Sprintf("De @%s  para  @%s", stripEmojiForFont(fromName), stripEmojiForFont(toName)))
	}
	drawCenteredText(img, fFoot, w/2, h-62, colPink, "LUNAR BOT  •  CANTADA")
	return encodePNG(img)
}

func consumeInventoryItem(inv []string, match string) ([]string, bool) {
	for i, it := range inv {
		if strings.Contains(strings.ToLower(it), strings.ToLower(match)) {
			out := make([]string, 0, len(inv)-1)
			out = append(out, inv[:i]...)
			out = append(out, inv[i+1:]...)
			return out, true
		}
	}
	return inv, false
}

func renderProfileCard(name, jidTag string, coins, xp, needXP, level, wins, losses int64, spouse string, inv []string, avatar []byte) ([]byte, error) {
	w, h := 880, 460
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colBG)
	fillRoundedRect(img, 24, 24, w-48, h-48, 24, colPurple)
	fillRoundedRect(img, 27, 27, w-54, h-54, 22, colCard)

	fTag := makeFace(true, 18)
	fName := makeFace(true, 32)
	fSub := makeFace(false, 22)
	fStat := makeFace(true, 26)
	fFoot := makeFace(false, 16)
	defer func() {
		for _, f := range []font.Face{fTag, fName, fSub, fStat, fFoot} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	drawCircularAvatar(img, avatar, 130, 140, 62, "L", colPurple)

	dispName := stripEmojiForFont(name)
	if dispName == "" {
		dispName = "@" + jidTag
	}
	if len(dispName) > 22 {
		dispName = dispName[:22] + "..."
	}
	drawText(img, fName, 224, 118, colText, dispName)
	drawText(img, fSub, 224, 152, colMuted, fmt.Sprintf("Nivel %d   •   %s", level, stripEmojiForFont(spouse)))

	// XP progress bar
	barX, barY, barW, barH := 224, 174, 590, 26
	fillRoundedRect(img, barX, barY, barW, barH, 12, colCardDark)
	pct := 0.0
	if needXP > 0 {
		pct = math.Min(1.0, float64(xp)/float64(needXP))
	}
	fw := int(float64(barW) * pct)
	if fw > 0 && fw < 20 {
		fw = 20
	}
	if fw > 0 {
		fillRoundedRect(img, barX, barY, fw, barH, 12, colCyan)
	}
	drawText(img, fFoot, barX, barY+46, colMuted, fmt.Sprintf("XP: %d / %d", xp, needXP))

	// Stat boxes
	boxes := []struct {
		lbl, val string
		col      color.RGBA
	}{
		{"LUNARCOINS", fmt.Sprintf("%d", coins), colAmber},
		{"VITORIAS", fmt.Sprintf("%d", wins), colEmerald},
		{"DERROTAS", fmt.Sprintf("%d", losses), colRose},
		{"ITENS", fmt.Sprintf("%d", len(inv)), colCyan},
	}
	bw := 182
	bx := 56
	for _, b := range boxes {
		fillRoundedRect(img, bx, 265, bw, 110, 14, colCardDark)
		drawCenteredText(img, fTag, bx+bw/2, 300, colMuted, b.lbl)
		drawCenteredText(img, fStat, bx+bw/2, 344, b.col, b.val)
		bx += bw + 14
	}

	drawText(img, fFoot, 56, h-44, colGrid, "LUNAR BOT  •  PERFIL")
	return encodePNG(img)
}

func renderCasamentoCard(rootDir, leftName, rightName string, pct int, leftAv, rightAv []byte) ([]byte, error) {
	bgPath := filepath.Join(rootDir, "assets", "casamento-bg.png")
	rawBG, err := os.ReadFile(bgPath)
	if err != nil {
		return renderVersusCard("CASAMENTO", "FELICIDADES AOS NOIVOS!", leftName, rightName, fmt.Sprintf("%d%%", pct), "Casamento oficializado!", leftAv, rightAv, colRose)
	}
	srcBG, _, err := image.Decode(bytes.NewReader(rawBG))
	if err != nil {
		return renderVersusCard("CASAMENTO", "FELICIDADES AOS NOIVOS!", leftName, rightName, fmt.Sprintf("%d%%", pct), "Casamento oficializado!", leftAv, rightAv, colRose)
	}

	b := srcBG.Bounds()
	w, h := b.Dx(), b.Dy()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), srcBG, b.Min, draw.Src)

	colNeon := color.RGBA{255, 46, 126, 255}
	colSoft := color.RGBA{251, 207, 232, 255}

	fTag := makeFace(true, 18)
	fTitle := makeFace(true, 30)
	fPct := makeFace(true, 60)
	fName := makeFace(true, 21)
	fBox1 := makeFace(true, 23)
	fBox2 := makeFace(false, 20)
	defer func() {
		for _, f := range []font.Face{fTag, fTitle, fPct, fName, fBox1, fBox2} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	fillRoundedRect(img, 240, 44, 175, 34, 15, colNeon)
	drawCenteredText(img, fTag, 327, 67, colText, "CASAMENTO")
	drawText(img, fTitle, 434, 71, colText, "COMPATIBILIDADE")

	// Circular avatars with neon pink ring
	drawCircleRing(img, 280, 204, 70, 5, colNeon)
	drawCircularAvatar(img, leftAv, 280, 204, 66, "1", colNeon)
	drawCenteredText(img, fName, 280, 304, colText, stripEmojiForFont(leftName))

	drawCircleRing(img, w-280, 204, 70, 5, colNeon)
	drawCircularAvatar(img, rightAv, w-280, 204, 66, "2", colNeon)
	drawCenteredText(img, fName, w-280, 304, colText, stripEmojiForFont(rightName))

	// Center percentage + hearts
	drawHeart(img, w/2-115, 202, 11, colNeon)
	drawCenteredText(img, fPct, w/2, 224, colNeon, fmt.Sprintf("%d%%", pct))
	drawHeart(img, w/2+115, 202, 11, colNeon)

	// Bottom text box overlay
	drawCenteredText(img, fBox1, w/2, 396, colText, fmt.Sprintf("Casamento oficializado com %d%% de compatibilidade!", pct))
	drawCenteredText(img, fBox2, w/2, 434, colSoft, "Com amor, respeito e parceria, felicidades aos noivos!")
	return encodePNG(img)
}

func renderVersusCard(tag, title, leftName, rightName, centerBadge, footerText string, leftAv, rightAv []byte, accent color.RGBA) ([]byte, error) {
	w, h := 880, 440
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colBG)
	fillRoundedRect(img, 24, 24, w-48, h-48, 24, accent)
	fillRoundedRect(img, 27, 27, w-54, h-54, 22, colCard)

	fTag := makeFace(true, 18)
	fTitle := makeFace(true, 30)
	fCenter := makeFace(true, 44)
	fName := makeFace(true, 22)
	fFoot := makeFace(false, 22)
	defer func() {
		for _, f := range []font.Face{fTag, fTitle, fCenter, fName, fFoot} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	fillRoundedRect(img, 54, 50, 190, 32, 10, accent)
	drawCenteredText(img, fTag, 149, 72, colBG, strings.ToUpper(stripEmojiForFont(tag)))
	drawText(img, fTitle, 264, 76, colText, stripEmojiForFont(title))

	drawCircularAvatar(img, leftAv, 210, 200, 68, "1", colCyan)
	drawCircularAvatar(img, rightAv, w-210, 200, 68, "2", accent)

	drawCenteredText(img, fCenter, w/2, 214, accent, stripEmojiForFont(centerBadge))
	drawCenteredText(img, fName, 210, 302, colText, stripEmojiForFont(leftName))
	drawCenteredText(img, fName, w-210, 302, colText, stripEmojiForFont(rightName))

	fillRoundedRect(img, 60, 336, w-120, 56, 14, colCardDark)
	drawCenteredText(img, fFoot, w/2, 371, colText, stripEmojiForFont(footerText))
	return encodePNG(img)
}

func makeStickerFace(rootDir string, size float64) font.Face {
	if raw, err := os.ReadFile(filepath.Join(rootDir, "assets", "sticker-font.ttf")); err == nil {
		if f, err := opentype.Parse(raw); err == nil {
			if face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull}); err == nil {
				return face
			}
		}
	}
	return makeFace(true, size)
}

func renderTextStickerFrames(rootDir, text string, bg, fg color.RGBA) ([][]byte, error) {
	const w, h = 512, 400
	f := makeStickerFace(rootDir, 62)
	if f == nil {
		return nil, fmt.Errorf("font unavailable")
	}
	defer f.Close()

	fullLines := wrapText(f, text, 456)
	if len(fullLines) == 0 {
		return nil, fmt.Errorf("empty text")
	}
	if len(fullLines) > 5 {
		fullLines = fullLines[:5]
	}

	totalRunes := 0
	lineRunes := make([][]rune, len(fullLines))
	for i, l := range fullLines {
		r := []rune(l)
		lineRunes[i] = r
		totalRunes += len(r)
	}
	if totalRunes == 0 {
		return nil, fmt.Errorf("empty text")
	}

	step := 1
	if totalRunes > 24 {
		step = (totalRunes + 23) / 24
	}

	var counts []int
	for c := step; c < totalRunes; c += step {
		counts = append(counts, c)
	}
	for k := 0; k < 6; k++ {
		counts = append(counts, totalRunes)
	}

	lineH := 68
	startY := (h-(len(fullLines)-1)*lineH)/2 + 20
	outline := color.RGBA{10, 10, 16, 255}

	var frames [][]byte
	for _, limit := range counts {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		fillRect(img, 0, 0, w, h, bg)
		rem := limit
		y := startY
		for i, lr := range lineRunes {
			if rem <= 0 {
				break
			}
			take := len(lr)
			if rem < take {
				take = rem
			}
			rem -= take
			partial := string(lr[:take])
			fullW := font.MeasureString(f, fullLines[i]).Ceil()
			x := (w - fullW) / 2
			for oy := -3; oy <= 3; oy++ {
				for ox := -3; ox <= 3; ox++ {
					if ox != 0 || oy != 0 {
						drawText(img, f, x+ox, y+oy, outline, partial)
					}
				}
			}
			drawText(img, f, x, y, fg, partial)
			y += lineH
		}
		pngBytes, err := encodePNG(img)
		if err != nil {
			return nil, err
		}
		frames = append(frames, pngBytes)
	}
	return frames, nil
}

func renderTextStickerPNG(text string, fg color.RGBA) ([]byte, error) {
	const size = 512
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	f := makeFace(true, 58)
	if f == nil {
		return nil, fmt.Errorf("font unavailable")
	}
	defer f.Close()

	lines := wrapText(f, text, 450)
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty text")
	}
	if len(lines) > 6 {
		lines = lines[:6]
	}

	lineH := 66
	startY := (size-(len(lines)-1)*lineH)/2 + 18
	outline := color.RGBA{10, 10, 16, 255}
	for _, line := range lines {
		w := font.MeasureString(f, line).Ceil()
		x := (size - w) / 2
		for oy := -3; oy <= 3; oy++ {
			for ox := -3; ox <= 3; ox++ {
				if ox != 0 || oy != 0 {
					drawText(img, f, x+ox, startY+oy, outline, line)
				}
			}
		}
		drawText(img, f, x, startY, fg, line)
		startY += lineH
	}
	return encodePNG(img)
}

func fetchAvatar(ctx *Context, j types.JID) []byte {
	if ctx == nil || ctx.Bot == nil || ctx.Bot.Raw() == nil || j.IsEmpty() {
		return nil
	}
	pctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	info, err := ctx.Bot.Raw().GetProfilePictureInfo(pctx, j, &whatsmeow.GetProfilePictureParams{Preview: true})
	if err != nil || info == nil || info.URL == "" {
		return nil
	}
	pic, err := httpGetBytes(info.URL)
	if err != nil {
		return nil
	}
	return pic
}

func sendCard(ctx *Context, img []byte, err error, caption string, mentions ...string) error {
	if err == nil && len(img) > 0 {
		return ctx.SendImage(img, caption, mentions...)
	}
	return ctx.Bot.SendText(ctx.From, caption, mentions...)
}

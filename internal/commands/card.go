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
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
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

func renderRankCard(title string, entries []rankEntry, accent color.RGBA) ([]byte, error) {
	fTag := makeFace(true, 18)
	fTitle := makeFace(true, 32)
	fPos := makeFace(true, 24)
	fName := makeFace(true, 23)
	fStat := makeFace(false, 21)
	fFoot := makeFace(false, 16)
	defer func() {
		for _, f := range []font.Face{fTag, fTitle, fPos, fName, fStat, fFoot} {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	if len(entries) > 10 {
		entries = entries[:10]
	}
	w := 880
	rowH := 76
	h := 210 + len(entries)*(rowH+10)
	if h < 320 {
		h = 320
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colBG)
	fillRoundedRect(img, 24, 24, w-48, h-48, 22, accent)
	fillRoundedRect(img, 27, 27, w-54, h-54, 20, colCard)

	fillRoundedRect(img, 54, 50, 210, 34, 10, accent)
	drawCenteredText(img, fTag, 159, 73, colBG, "RANKING LUNAR")
	drawText(img, fTitle, 54, 128, colText, stripEmojiForFont(title))

	y := 156
	for _, e := range entries {
		fillRoundedRect(img, 52, y, w-104, rowH, 14, colCardDark)
		posCol := colMuted
		switch e.Pos {
		case 1:
			posCol = colAmber
		case 2:
			posCol = color.RGBA{203, 213, 225, 255}
		case 3:
			posCol = color.RGBA{217, 119, 6, 255}
		}
		drawCenteredText(img, fPos, 88, y+46, posCol, fmt.Sprintf("#%d", e.Pos))

		drawCircularAvatar(img, e.Avatar, 152, y+rowH/2, 26, fmt.Sprintf("%d", e.Pos), posCol)

		name := stripEmojiForFont(e.Name)
		if len(name) > 24 {
			name = name[:24] + "..."
		}
		drawText(img, fName, 198, y+45, colText, name)

		stat := stripEmojiForFont(e.Stat)
		sw := 0
		if fStat != nil {
			sw = font.MeasureString(fStat, stat).Ceil()
		}
		drawText(img, fStat, w-76-sw, y+45, accent, stat)
		y += rowH + 10
	}

	drawText(img, fFoot, 54, h-44, colGrid, "LUNAR BOT  •  TOP RANKING")
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

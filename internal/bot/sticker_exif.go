package bot

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
)

// injectWebPExif embeds WhatsApp sticker pack metadata (EXIF chunk) into a WebP RIFF container.
func injectWebPExif(webp []byte, packname, author string) []byte {
	if len(webp) < 12 || string(webp[0:4]) != "RIFF" || string(webp[8:12]) != "WEBP" {
		return webp
	}
	if packname == "" {
		packname = "BY LCSX"
	}
	if author == "" {
		author = "BY LCSX"
	}

	meta, err := json.Marshal(map[string]any{
		"sticker-pack-id":        "com.lunarbot.bylcsx",
		"sticker-pack-name":      packname,
		"sticker-pack-publisher": author,
		"emojis":                 []string{"🌙"},
	})
	if err != nil {
		return webp
	}

	exif := make([]byte, 22+len(meta))
	copy(exif[:22], []byte{
		0x49, 0x49, 0x2A, 0x00,
		0x08, 0x00, 0x00, 0x00,
		0x01, 0x00,
		0x41, 0x57, 0x07, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x16, 0x00, 0x00, 0x00,
	})
	binary.LittleEndian.PutUint32(exif[14:18], uint32(len(meta)))
	copy(exif[22:], meta)

	// Parse existing RIFF chunks, strip old EXIF, and ensure VP8X exists with EXIF flag (0x08).
	type chunk struct {
		id   string
		data []byte
	}
	var chunks []chunk
	hasVP8X := false
	hasAnim := false
	hasAlpha := false

	pos := 12
	for pos+8 <= len(webp) {
		id := string(webp[pos : pos+4])
		sz := int(binary.LittleEndian.Uint32(webp[pos+4 : pos+8]))
		end := pos + 8 + sz
		if end > len(webp) {
			return webp
		}
		payload := make([]byte, sz)
		copy(payload, webp[pos+8:end])
		pos = end
		if sz%2 == 1 && pos < len(webp) {
			pos++
		}
		if id == "EXIF" {
			continue
		}
		if id == "VP8X" && len(payload) >= 4 {
			hasVP8X = true
			payload[0] |= 0x08
		}
		if id == "ANIM" {
			hasAnim = true
		}
		if id == "ALPH" {
			hasAlpha = true
		}
		chunks = append(chunks, chunk{id: id, data: payload})
	}

	if !hasVP8X {
		vp8x := make([]byte, 10)
		var flags byte = 0x08 // EXIF
		if hasAnim {
			flags |= 0x02
		}
		if hasAlpha {
			flags |= 0x10
		}
		vp8x[0] = flags
		// 512x512 stored as (dim - 1) in 24-bit little endian
		const d = 511
		vp8x[4], vp8x[5], vp8x[6] = byte(d&0xFF), byte((d>>8)&0xFF), byte((d>>16)&0xFF)
		vp8x[7], vp8x[8], vp8x[9] = byte(d&0xFF), byte((d>>8)&0xFF), byte((d>>16)&0xFF)
		chunks = append([]chunk{{id: "VP8X", data: vp8x}}, chunks...)
	}
	chunks = append(chunks, chunk{id: "EXIF", data: exif})

	var body bytes.Buffer
	body.WriteString("WEBP")
	for _, ch := range chunks {
		body.WriteString(ch.id)
		var szBuf [4]byte
		binary.LittleEndian.PutUint32(szBuf[:], uint32(len(ch.data)))
		body.Write(szBuf[:])
		body.Write(ch.data)
		if len(ch.data)%2 == 1 {
			body.WriteByte(0)
		}
	}

	out := make([]byte, 8+body.Len())
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(body.Len()))
	copy(out[8:], body.Bytes())
	return out
}

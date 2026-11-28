package bot

import (
	"bytes"
	"testing"
)

func TestInjectWebPExif(t *testing.T) {
	// Minimal valid RIFF/WEBP with a dummy VP8 chunk
	src := []byte{
		'R', 'I', 'F', 'F', 16, 0, 0, 0,
		'W', 'E', 'B', 'P',
		'V', 'P', '8', ' ', 4, 0, 0, 0, 1, 2, 3, 4,
	}
	out := injectWebPExif(src, "BY LCSX", "BY LCSX")
	if string(out[0:4]) != "RIFF" || string(out[8:12]) != "WEBP" {
		t.Fatalf("invalid RIFF/WEBP header")
	}
	if !bytes.Contains(out, []byte("VP8X")) {
		t.Fatalf("expected VP8X chunk")
	}
	if !bytes.Contains(out, []byte("EXIF")) || !bytes.Contains(out, []byte("BY LCSX")) {
		t.Fatalf("expected EXIF chunk with BY LCSX")
	}
}

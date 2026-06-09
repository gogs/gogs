package tool

import (
	"testing"

	"gogs.io/gogs/internal/conf"
)

func TestDetectEncodingUsesANSICharsetForNonUTF8(t *testing.T) {
	conf.Repository.ANSICharset = "windows-1252"
	t.Cleanup(func() {
		conf.Repository.ANSICharset = ""
	})

	charset, err := DetectEncoding([]byte{0x80})
	if err != nil {
		t.Fatal(err)
	}
	if charset != "windows-1252" {
		t.Fatalf("charset: want %q but got %q", "windows-1252", charset)
	}
}

func TestDetectEncodingReturnsErrorWhenCharsetNotDetected(t *testing.T) {
	conf.Repository.ANSICharset = ""
	t.Cleanup(func() {
		conf.Repository.ANSICharset = ""
	})

	charset, err := DetectEncoding([]byte{0x80})
	if err == nil {
		t.Fatal("expected error")
	}
	if charset != "" {
		t.Fatalf("charset: want empty but got %q", charset)
	}
}

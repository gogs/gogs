package chardet

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/unicode/norm"
)

// These tests are ported from the `mod tests` of chardetng's src/lib.rs and
// are the same as in the parent package.

func checkBytes(t *testing.T, input []byte, want string) {
	t.Helper()
	det := New()
	det.Feed(input, true)
	require.Equal(t, want, det.Guess(false))
}

// decomposeVietneseTones is the port of detone's
// decompose_vietnamese_tones(true): every tone mark becomes a combining
// character after its base, while circumflex, breve and horn stay attached.
func decomposeVietneseTones(s string) string {
	var out strings.Builder
	for _, r := range s {
		var base []rune
		var tone rune
		for _, d := range norm.NFD.String(string(r)) {
			switch d {
			case 0x0300, 0x0301, 0x0303, 0x0309, 0x0323:
				tone = d
			default:
				base = append(base, d)
			}
		}
		out.WriteString(norm.NFC.String(string(base)))
		if tone != 0 {
			out.WriteRune(tone)
		}
	}
	return out.String()
}

func check(t *testing.T, input, want string) {
	t.Helper()
	if want == "windows-1258" {
		input = decomposeVietneseTones(input)
	}
	enc, err := htmlindex.Get(want)
	require.NoError(t, err)
	// Like encoding_rs's Encoding::encode, replace unmappable characters
	// with decimal numeric character references.
	encoded, err := encoding.HTMLEscapeUnsupported(enc.NewEncoder()).Bytes([]byte(input))
	require.NoError(t, err)
	checkBytes(t, encoded, want)
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name, input, encoding string
	}{
		{"fi", "Ääni", "windows-1252"},
		{"fi_bis", "Tämä", "windows-1252"},
		{"pt", "Este é um teste de codificação de caracteres.", "windows-1252"},
		{"is", "Þetta er kóðunarpróf á staf. Fyrir sum tungumál sem nota latneska stafi þurfum við meira inntak til að taka ákvörðunina.", "windows-1252"},
		{"ru_short", "Русский", "windows-1251"},
		{"ru", "Это тест кодировки символов.", "windows-1251"},
		{"ru_iso", "Это тест кодировки символов.", "ISO-8859-5"},
		{"ru_ibm", "Это тест кодировки символов.", "IBM866"},
		{"ru_koi", "Это тест кодировки символов.", "KOI8-U"},
		{"uk", "Це тест на кодування символів.", "windows-1251"},
		{"uk_koi", "Це тест на кодування символів.", "KOI8-U"},
		{"el_short", "Ελληνικά", "windows-1253"},
		{"el", "Πρόκειται για δοκιμή κωδικοποίησης χαρακτήρων: Άρης", "windows-1253"},
		{"el_iso", "Πρόκειται για δοκιμή κωδικοποίησης χαρακτήρων: Άρης", "ISO-8859-7"},
		{"de", "Straße", "windows-1252"},
		{"he", "\u05E2\u05D1\u05E8\u05D9\u05EA", "windows-1255"},
		{"2022", "日本語", "ISO-2022-JP"},
		{"th", "นี่คือการทดสอบการเข้ารหัสอักขระ", "windows-874"},
		{"vi", "Đây là một thử nghiệm mã hóa ký tự.", "windows-1258"},
		{"tr", "Bu bir karakter kodlama testidir. Latince karakterleri kullanan bazı dillerde karar vermek için daha fazla girdiye ihtiyacımız var.", "windows-1254"},
		{"simplified", "这是一个字符编码测试。", "GBK"},
		{"traditional", "這是一個字符編碼測試。", "Big5"},
		{"ko", "이것은 문자 인코딩 테스트입니다.", "EUC-KR"},
		{"shift", "これは文字実験です。", "Shift_JIS"},
		{"euc", "これは文字実験です。", "EUC-JP"},
		{"ar", "هذا هو اختبار ترميز الأحرف.", "windows-1256"},
		{"ar_iso", "هذا هو اختبار ترميز الأحرف.", "ISO-8859-6"},
		{"fa", "این یک تست رمزگذاری کاراکتر است.", "windows-1256"},
		{"visual", ".םיוות דודיק ןחבמ והז", "ISO-8859-8"},
		{"yi", "דאָס איז אַ טעסט פֿאַר קאָדירונג פון כאַראַקטער.", "windows-1255"},
		{"it", "è", "windows-1252"},
		{"en", "isn’t", "windows-1252"},
		{"en_bis", "Rock ’n Roll", "windows-1252"},
		{"ca", "Codificació de caràcters", "windows-1252"},
		{"et", "või", "windows-1252"},
		{"pl_iso", "To jest test kodowania znaków. W przypadku niektórych języków, które używają znaków łacińskich, potrzebujemy więcej danych, aby podjąć decyzję.", "ISO-8859-2"},
		{"pl", "To jest test kodowania znaków. W przypadku niektórych języków, które używają znaków łacińskich, potrzebujemy więcej danych, aby podjąć decyzję.", "windows-1250"},
		{"lt", "Tai simbolių kodavimo testas. Kai kurioms kalboms, naudojančioms lotyniškus rašmenis, mums reikia daugiau informacijos, kad galėtume priimti sprendimą.", "windows-1257"},
		{"lv", "Šis ir rakstzīmju kodēšanas tests. Dažās valodās, kurās tiek izmantotas latīņu valodas burti, lēmuma pieņemšanai mums ir nepieciešams vairāk ieguldījuma.", "windows-1257"},
		{"lv_iso_8859_4", "Šis ir rakstzīmju kodēšanas tests. Dažās valodās, kurās tiek izmantotas latīņu valodas burti, lēmuma pieņemšanai mums ir nepieciešams vairāk ieguldījuma.", "ISO-8859-4"},
		{"a0", "\u00A0\u00A0 \u00A0", "windows-1252"},
		{"a0a0", "\u00A0\u00A0", "windows-1252"},
		{"space_copyright_space", " © ", "windows-1252"},
		{"space_masculine_space", " º ", "windows-1252"},
		{"space_feminine_space", " ª ", "windows-1252"},
		{"period_masculine_space", ".º ", "windows-1252"},
		{"period_feminine_space", ".ª ", "windows-1252"},
		{"maria", " Mª ", "windows-1252"},
		{"dona", " Dª ", "windows-1252"},
		{"nuestra", " Nª ", "windows-1252"},
		{"senora", " Sª ", "windows-1252"},
		{"digit_feminine", " 42ª ", "windows-1252"},
		{"digit_masculine", " 42º ", "windows-1252"},
		{"roman_feminine", " XIVª ", "windows-1252"},
		{"roman_masculine", " XIVº ", "windows-1252"},
		{"numero_uno", "Nº1", "windows-1252"},
		{"numero", "Nº", "windows-1252"},
		{"euro", " €9", "windows-1252"},
		{"shift_jis_half_width_katakana", "ﾊｰﾄﾞｳｪｱﾊｰﾄﾞｳｪｱﾊｰﾄﾞｳｪｱﾊｰﾄﾞｳｪｱﾊｰﾄﾞｳｪｱ", "Shift_JIS"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check(t, test.input, test.encoding)
		})
	}
}

func TestCheckBytes(t *testing.T) {
	repeat := func(prefix string, unit string, n int, suffix string) []byte {
		return []byte(prefix + strings.Repeat(unit, n) + suffix)
	}
	const korean = "\xC5\xD7\xBD\xBA\xC6\xAE. "
	const sjisKana = "\x82\xC9\x82\xD9\x82\xF1\x82\xB2"
	const eucJPKana = "\xA4\xCB\xA4\xDB\xA4\xF3\xA4\xB4"
	tests := []struct {
		name     string
		input    []byte
		encoding string
	}{
		{"en_windows1252", []byte{68, 111, 110, 180, 116, 32}, "windows-1252"},
		{"i_apostrophe", []byte("I\x92"), "windows-1252"},
		{"big5_pua", repeat("", "\xA4\x40", 40, "\x81\x40\xA4\x40"), "Big5"},
		{"big5_single_byte_a0", repeat("", "\xA4\x40", 80, "\x81\x40\xA0 "), "Big5"},
		{"big5_single_byte_ff", repeat("", "\xA4\x40", 80, "\x81\x40\xFF "), "Big5"},
		{"not_big5", repeat("", "\xA4\x40", 40, "\x81\x40\xA0\xA0"), "IBM866"},
		{"euc_kr_pua", repeat("\xC9\xA1\xB0\xA1 ", korean, 40, ""), "EUC-KR"},
		{"euc_kr_pua_bis", repeat("\xFE\xA1\xB0\xA1 ", korean, 40, ""), "EUC-KR"},
		{"euc_kr_single_byte_ff", repeat("\xFF ", korean, 40, ""), "EUC-KR"},
		{"euc_kr_single_byte_81", repeat("\x81 ", korean, 40, ""), "EUC-KR"},
		{"euc_kr_single_byte_84", repeat("\x84 ", korean, 40, ""), "EUC-KR"},
		{"not_euc_kr", repeat("\xC9\xA0\xB0\xA1 ", korean, 40, ""), "GBK"},
		{"shift_jis_x0213", repeat("\x87\xE5", sjisKana, 40, ""), "Shift_JIS"},
		{"shift_jis_single_byte_fd", repeat("\xFD", sjisKana, 40, ""), "Shift_JIS"},
		{"not_shift_jis", repeat("\x84\xE0", sjisKana, 40, ""), "GBK"},
		{"not_shift_jis_bis", repeat("\x87\x7D", sjisKana, 40, ""), "GBK"},
		{"euc_jp_x0213", repeat("\xAD\xBF", eucJPKana, 80, ""), "EUC-JP"},
		{"euc_jp_x0213_other_plane", repeat("\x8F\xFE\xF6", eucJPKana, 80, ""), "EUC-JP"},
		{"not_euc_jp", repeat("\x8F\xFE\xF7", eucJPKana, 80, ""), "windows-1252"},
		{"not_euc_jp_bis", repeat("\xA8\xDF", eucJPKana, 80, ""), "Big5"},
		{"gbk_single_byte_ff", repeat("\xFF", "\xB5\xC4", 80, ""), "GBK"},
		{"gbk_single_byte_a0", repeat("\xA0 ", "\xB5\xC4", 80, ""), "GBK"},
		{"gbk_single_byte_fe", repeat("\xFE ", "\xB5\xC4", 80, ""), "GBK"},
		{"not_gbk_single_byte_fc", repeat("\xFC ", "\xB5\xC4", 80, ""), "ISO-8859-5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkBytes(t, test.input, test.encoding)
		})
	}
}

func TestStreamingNumero(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
	}{
		{"one_by_one", []string{"n", ".", "\xBA", "1"}},
		{"two_together", []string{"n.", "\xBA", "1"}},
		{"one_by_one_extra_before", []string{" n", ".", "\xBA", "1"}},
		{"one_before", []string{"n", ".\xBA", "1"}},
		{"longer_first_buffer", []string{"rrn.", "\xBA", "1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			det := New()
			for i, chunk := range test.chunks {
				det.Feed([]byte(chunk), i == len(test.chunks)-1)
			}
			require.Equal(t, "windows-1252", det.Guess(false))
		})
	}
}

func TestEmpty(t *testing.T) {
	det := New()
	seenNonASCII := det.Feed(nil, true)
	require.Equal(t, "windows-1252", det.Guess(false))
	require.False(t, seenNonASCII)
}

func TestNo2022(t *testing.T) {
	enc, err := htmlindex.Get("ISO-2022-JP")
	require.NoError(t, err)
	encoded, err := enc.NewEncoder().Bytes([]byte("日本語"))
	require.NoError(t, err)
	det := New()
	// The port of EncodingDetector::new(Iso2022JpDetection::Deny).
	det.isoAlive = false
	det.Feed(encoded, true)
	require.Equal(t, "windows-1252", det.Guess(false))
}

// The tests below are not from chardetng.

func TestGuessNamesWorkWithCharsetLookup(t *testing.T) {
	names := map[string]bool{"UTF-8": true, "ISO-2022-JP": true, "GBK": true, "EUC-JP": true, "EUC-KR": true, "Shift_JIS": true, "Big5": true}
	for _, d := range sbData {
		names[d.encoding] = true
	}
	for name := range names {
		enc, canonical := charset.Lookup(name)
		require.NotNil(t, enc, name)
		require.Equal(t, strings.ToLower(name), strings.ToLower(canonical), name)
	}
}

func TestAllowUTF8(t *testing.T) {
	det := New()
	det.Feed([]byte("Ääni"), true)
	require.Equal(t, "UTF-8", det.Guess(true))
	require.Equal(t, "windows-1252", det.Guess(false))
}

// TestTwoByteDecodersMatchXText checks every lead and trail byte pair of the
// byte-at-a-time decoders against golang.org/x/text, which decodes errors to
// U+FFFD. x/text does not implement some WHATWG mappings (e.g., the PUA and
// GB18030-2022 code points), so a pair x/text decodes to U+FFFD is only
// counted, not compared.
func TestTwoByteDecodersMatchXText(t *testing.T) {
	type decoder interface{ decodeByte(b byte) decodeStep }
	tests := []struct {
		encoding   string
		newDecoder func() decoder
	}{
		{"Big5", func() decoder { return &big5Decoder{} }},
		{"EUC-KR", func() decoder { return &eucKRDecoder{} }},
		{"Shift_JIS", func() decoder { return &shiftJISDecoder{} }},
		{"EUC-JP", func() decoder { return &eucJPDecoder{} }},
		{"GBK", func() decoder { return &gb18030Decoder{} }},
	}
	for _, test := range tests {
		t.Run(test.encoding, func(t *testing.T) {
			enc, err := htmlindex.Get(test.encoding)
			require.NoError(t, err)
			var mismatches []string
			xtextGaps := 0
			for lead := 0x81; lead <= 0xFE; lead++ {
				for trail := 0x40; trail <= 0xFE; trail++ {
					d := test.newDecoder()
					if first := d.decodeByte(byte(lead)); first.malformed || first.written != 0 {
						continue
					}
					step := d.decodeByte(byte(trail))
					want, err := enc.NewDecoder().Bytes([]byte{byte(lead), byte(trail)})
					require.NoError(t, err)
					xtextError := strings.HasPrefix(string(want), "\uFFFD")
					if step.malformed {
						if !xtextError {
							mismatches = append(mismatches, fmt.Sprintf("%02X%02X: malformed, x/text %q", lead, trail, want))
						}
						continue
					}
					if step.written == 0 {
						continue // GB18030 four-byte sequence in progress
					}
					var units []rune
					for i := 0; i < step.written; i++ {
						units = append(units, rune(step.dst[i]))
					}
					got := string(utf16Decode(units))
					if xtextError {
						xtextGaps++
						continue
					}
					if !bytes.Equal([]byte(got), want) {
						mismatches = append(mismatches, fmt.Sprintf("%02X%02X: %q != x/text %q", lead, trail, got, want))
					}
				}
			}
			t.Logf("pairs that x/text decodes to U+FFFD but WHATWG maps: %d", xtextGaps)
			require.Empty(t, mismatches)
		})
	}
}

func utf16Decode(units []rune) []rune {
	if len(units) == 2 && units[0] >= 0xD800 && units[0] < 0xDC00 {
		return []rune{0x10000 + (units[0]-0xD800)<<10 + (units[1] - 0xDC00)}
	}
	return units
}

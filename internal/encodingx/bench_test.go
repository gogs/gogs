package encodingx

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/htmlindex"
)

var benchInputs = []struct {
	name, encoding, text string
	repeat               int
}{
	{"GBK", "GBK", "// 这个函数用于计算用户的权限，并返回结果。\n// 如果用户不存在，则返回错误信息。\nfunc 检查权限() {}\n", 2000},
	{"windows-1251", "windows-1251", "Это тестовый файл на русском языке. Проверяем правильность определения кодировки.", 2500},
	{"Shift_JIS", "Shift_JIS", "// この関数はユーザーの権限を確認します。\n// ユーザーが存在しない場合はエラーを返します。\n", 2200},
	{"windows-1252", "windows-1252", "Ceci est un fichier de test en français. L'été dernier, nous sommes allés à la plage, c'était génial !\n", 2000},
	{"short", "windows-1252", "Fix the café menu rendering.", 1},
}

func benchmarkDetect(b *testing.B, detect func([]byte) string) {
	// Build the lazily built tables outside of the measurement.
	detect([]byte("\xff"))
	for _, in := range benchInputs {
		enc, err := htmlindex.Get(in.encoding)
		if err != nil {
			b.Fatal(err)
		}
		data, err := enc.NewEncoder().String(strings.Repeat(in.text, in.repeat))
		if err != nil {
			b.Fatal(err)
		}
		input := []byte(data)
		b.Run(in.name, func(b *testing.B) {
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			for b.Loop() {
				if detect(input) != in.encoding {
					b.Fatal("wrong guess")
				}
			}
		})
	}
}

func BenchmarkDetect(b *testing.B) {
	benchmarkDetect(b, func(input []byte) string {
		d := New()
		d.Feed(input, true)
		return d.Guess(false)
	})
}

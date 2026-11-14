package template

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"unknwon.dev/x/logx"
)

func TestFuncMap(t *testing.T) {
	loggerType := reflect.TypeFor[*logx.Logger]()
	for _, funcs := range FuncMap(logx.NewNoopLogger()) {
		for name, fn := range funcs {
			typ := reflect.TypeOf(fn)
			for i := range typ.NumIn() {
				assert.NotEqual(t, loggerType, typ.In(i), "%s takes a logger, which templates cannot pass", name)
			}
		}
	}
}

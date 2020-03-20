// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ipynbSanitizer(t *testing.T) {
	p := ipynbSanitizer()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "allow 'class' and 'data-prompt-number' attributes",
			input: `
<div class="nb-notebook">
    <div class="nb-worksheet">
        <div class="nb-cell nb-markdown-cell">Hello world</div>
        <div class="nb-cell nb-code-cell">
            <div class="nb-input" data-prompt-number="4">
            </div>
        </div>
    </div>
</div>
`,
			want: `
<div class="nb-notebook">
    <div class="nb-worksheet">
        <div class="nb-cell nb-markdown-cell">Hello world</div>
        <div class="nb-cell nb-code-cell">
            <div class="nb-input" data-prompt-number="4">
            </div>
        </div>
    </div>
</div>
`,
		},
		{
			name: "allow base64 encoded images",
			input: `
<div class="nb-output" data-prompt-number="4">
    <img class="nb-image-output" src="data:image/png;base64,iVBORw0KGgoA"/>
</div>
`,
			want: `
<div class="nb-output" data-prompt-number="4">
    <img class="nb-image-output" src="data:image/png;base64,iVBORw0KGgoA"/>
</div>
`,
		},
		{
			name: "prevent XSS",
			input: `
<div class="nb-output" data-prompt-number="10">
<div class="nb-html-output">
<style>
.output {
align-items: center;
background: #00ff00;
}
</style>
<script>
function test() {
alert("test");
}

$(document).ready(test);
</script>
</div>
</div>
`,
			want: `
<div class="nb-output" data-prompt-number="10">
<div class="nb-html-output">


</div>
</div>
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, p.Sanitize(test.input))
		})
	}
}

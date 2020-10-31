// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

//go:generate go-bindata -nomemcopy -nometadata -pkg=conf -ignore="\\.DS_Store|README.md|TRANSLATORS|auth.d" -prefix=../../../ -debug=false -o=conf_gen.go ../../../conf/...

// Package sqlite registers the SQLite database driver and provides the GORM
// dialector for it.
//
// The engine is github.com/liustack/sqlite on the platforms it supports, and
// github.com/glebarez/go-sqlite on the others. Both register the driver as
// "sqlite", so exactly one of them is built into a binary.
//
// The dialector is derived from github.com/glebarez/sqlite v1.11.0, which
// imports the driver it registers and therefore cannot be used together with
// another one. It is under the MIT license:
//
//	Copyright (c) 2013-NOW  Jinzhu <wosmvp@gmail.com>
//
//	Permission is hereby granted, free of charge, to any person obtaining a copy
//	of this software and associated documentation files (the "Software"), to deal
//	in the Software without restriction, including without limitation the rights
//	to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
//	copies of the Software, and to permit persons to whom the Software is
//	furnished to do so, subject to the following conditions:
//
//	The above copyright notice and this permission notice shall be included in
//	all copies or substantial portions of the Software.
//
//	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
//	IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
//	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
//	AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
//	LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
//	OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
//	THE SOFTWARE.
package sqlite

// Extended result codes, see https://sqlite.org/rescode.html.
const (
	codeConstraintForeignKey = 787
	codeConstraintPrimaryKey = 1555
	codeConstraintUnique     = 2067
)

// IsUniqueViolation returns true when the error is a write that violates a
// unique index or the primary key.
func IsUniqueViolation(err error) bool {
	code := extendedCode(err)
	return code == codeConstraintPrimaryKey || code == codeConstraintUnique
}

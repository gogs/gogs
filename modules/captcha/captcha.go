// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package captcha a middleware that provides captcha service for Macaron.
package captcha

import (
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strings"

	"github.com/Unknwon/macaron"

	"github.com/gogits/cache"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

var (
	defaultChars = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
)

const (
	// default captcha attributes
	challengeNums    = 6
	expiration       = 600
	fieldIdName      = "captcha_id"
	fieldCaptchaName = "captcha"
	cachePrefix      = "captcha_"
	defaultURLPrefix = "/captcha/"
)

// Captcha struct
type Captcha struct {
	store cache.Cache

	// url prefix for captcha image
	URLPrefix string

	// specify captcha id input field name
	FieldIdName string
	// specify captcha result input field name
	FieldCaptchaName string

	// captcha image width and height
	StdWidth  int
	StdHeight int

	// captcha chars nums
	ChallengeNums int

	// captcha expiration seconds
	Expiration int64

	// cache key prefix
	CachePrefix string
}

// generate key string
func (c *Captcha) key(id string) string {
	return c.CachePrefix + id
}

// generate rand chars with default chars
func (c *Captcha) genRandChars() []byte {
	return base.RandomCreateBytes(c.ChallengeNums, defaultChars...)
}

// beego filter handler for serve captcha image
func (c *Captcha) Handler(ctx *macaron.Context) {
	var chars []byte

	id := path.Base(ctx.Req.RequestURI)
	if i := strings.Index(id, "."); i != -1 {
		id = id[:i]
	}

	key := c.key(id)

	if v, ok := c.store.Get(key).([]byte); ok {
		chars = v
	} else {
		ctx.Status(404)
		ctx.Write([]byte("captcha not found"))
		return
	}

	// reload captcha
	if len(ctx.Query("reload")) > 0 {
		chars = c.genRandChars()
		if err := c.store.Put(key, chars, c.Expiration); err != nil {
			ctx.Status(500)
			ctx.Write([]byte("captcha reload error"))
			log.Error(4, "Reload Create Captcha Error: %v", err)
			return
		}
	}

	img := NewImage(chars, c.StdWidth, c.StdHeight)
	if _, err := img.WriteTo(ctx.RW()); err != nil {
		log.Error(4, "Write Captcha Image Error: %v", err)
	}
}

// tempalte func for output html
func (c *Captcha) CreateCaptchaHtml() template.HTML {
	value, err := c.CreateCaptcha()
	if err != nil {
		log.Error(4, "Create Captcha Error: %v", err)
		return ""
	}

	// create html
	return template.HTML(fmt.Sprintf(`<input type="hidden" name="%s" value="%s">`+
		`<a class="captcha" href="javascript:">`+
		`<img onclick="this.src=('%s%s.png?reload='+(new Date()).getTime())" class="captcha-img" src="%s%s.png">`+
		`</a>`, c.FieldIdName, value, c.URLPrefix, value, c.URLPrefix, value))
}

// create a new captcha id
func (c *Captcha) CreateCaptcha() (string, error) {
	// generate captcha id
	id := string(base.RandomCreateBytes(15))

	// get the captcha chars
	chars := c.genRandChars()

	// save to store
	if err := c.store.Put(c.key(id), chars, c.Expiration); err != nil {
		return "", err
	}

	return id, nil
}

// verify from a request
func (c *Captcha) VerifyReq(req *http.Request) bool {
	req.ParseForm()
	return c.Verify(req.Form.Get(c.FieldIdName), req.Form.Get(c.FieldCaptchaName))
}

// direct verify id and challenge string
func (c *Captcha) Verify(id string, challenge string) (success bool) {
	if len(challenge) == 0 || len(id) == 0 {
		return
	}

	var chars []byte

	key := c.key(id)

	if v, ok := c.store.Get(key).([]byte); ok && len(v) == len(challenge) {
		chars = v
	} else {
		return
	}

	defer func() {
		// finally remove it
		c.store.Delete(key)
	}()

	// verify challenge
	for i, c := range chars {
		if c != challenge[i]-48 {
			return
		}
	}

	return true
}

// create a new captcha.Captcha
func NewCaptcha(urlPrefix string, store cache.Cache) *Captcha {
	cpt := &Captcha{}
	cpt.store = store
	cpt.FieldIdName = fieldIdName
	cpt.FieldCaptchaName = fieldCaptchaName
	cpt.ChallengeNums = challengeNums
	cpt.Expiration = expiration
	cpt.CachePrefix = cachePrefix
	cpt.StdWidth = stdWidth
	cpt.StdHeight = stdHeight

	if len(urlPrefix) == 0 {
		urlPrefix = defaultURLPrefix
	}

	if urlPrefix[len(urlPrefix)-1] != '/' {
		urlPrefix += "/"
	}

	cpt.URLPrefix = urlPrefix

	base.TemplateFuncs["CreateCaptcha"] = cpt.CreateCaptchaHtml
	return cpt
}

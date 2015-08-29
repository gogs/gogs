// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// for www.gravatar.com image cache

/*
It is recommend to use this way

	cacheDir := "./cache"
	defaultImg := "./default.jpg"
	http.Handle("/avatar/", avatar.CacheServer(cacheDir, defaultImg))
*/
package avatar

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nfnt/resize"

	"github.com/gogits/gogs/modules/identicon"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

var gravatarSource string

func UpdateGravatarSource() {
	gravatarSource = setting.GravatarSource
	if strings.HasPrefix(gravatarSource, "//") {
		gravatarSource = "http:" + gravatarSource
	} else if !strings.HasPrefix(gravatarSource, "http://") &&
		!strings.HasPrefix(gravatarSource, "https://") {
		gravatarSource = "http://" + gravatarSource
	}
	log.Debug("avatar.UpdateGravatarSource(update gavatar source): %s", gravatarSource)
}

// hash email to md5 string
// keep this func in order to make this package independent
func HashEmail(email string) string {
	// https://en.gravatar.com/site/implement/hash/
	email = strings.TrimSpace(email)
	email = strings.ToLower(email)

	h := md5.New()
	h.Write([]byte(email))
	return hex.EncodeToString(h.Sum(nil))
}

const _RANDOM_AVATAR_SIZE = 200

// RandomImage generates and returns a random avatar image.
func RandomImage(data []byte) (image.Image, error) {
	randExtent := len(palette.WebSafe) - 32
	rand.Seed(time.Now().UnixNano())
	colorIndex := rand.Intn(randExtent)
	backColorIndex := colorIndex - 1
	if backColorIndex < 0 {
		backColorIndex = randExtent - 1
	}

	// Size, background, forecolor
	imgMaker, err := identicon.New(_RANDOM_AVATAR_SIZE,
		palette.WebSafe[backColorIndex], palette.WebSafe[colorIndex:colorIndex+32]...)
	if err != nil {
		return nil, err
	}
	return imgMaker.Make(data), nil
}

// Avatar represents the avatar object.
type Avatar struct {
	Hash           string
	AlterImage     string // image path
	cacheDir       string // image save dir
	reqParams      string
	imagePath      string
	expireDuration time.Duration
}

func New(hash string, cacheDir string) *Avatar {
	return &Avatar{
		Hash:           hash,
		cacheDir:       cacheDir,
		expireDuration: time.Minute * 10,
		reqParams: url.Values{
			"d":    {"retro"},
			"size": {"200"},
			"r":    {"pg"}}.Encode(),
		imagePath: filepath.Join(cacheDir, hash+".image"), //maybe png or jpeg
	}
}

func (this *Avatar) HasCache() bool {
	fileInfo, err := os.Stat(this.imagePath)
	return err == nil && fileInfo.Mode().IsRegular()
}

func (this *Avatar) Modtime() (modtime time.Time, err error) {
	fileInfo, err := os.Stat(this.imagePath)
	if err != nil {
		return
	}
	return fileInfo.ModTime(), nil
}

func (this *Avatar) Expired() bool {
	modtime, err := this.Modtime()
	return err != nil || time.Since(modtime) > this.expireDuration
}

// default image format: jpeg
func (this *Avatar) Encode(wr io.Writer, size int) (err error) {
	var img image.Image
	decodeImageFile := func(file string) (img image.Image, err error) {
		fd, err := os.Open(file)
		if err != nil {
			return
		}
		defer fd.Close()

		if img, err = jpeg.Decode(fd); err != nil {
			fd.Seek(0, os.SEEK_SET)
			img, err = png.Decode(fd)
		}
		return
	}
	imgPath := this.imagePath
	if !this.HasCache() {
		if this.AlterImage == "" {
			return errors.New("request image failed, and no alt image offered")
		}
		imgPath = this.AlterImage
	}

	if img, err = decodeImageFile(imgPath); err != nil {
		return
	}
	m := resize.Resize(uint(size), 0, img, resize.NearestNeighbor)
	return jpeg.Encode(wr, m, nil)
}

// get image from gravatar.com
func (this *Avatar) Update() {
	UpdateGravatarSource()
	thunder.Fetch(gravatarSource+this.Hash+"?"+this.reqParams,
		this.imagePath)
}

func (this *Avatar) UpdateTimeout(timeout time.Duration) (err error) {
	UpdateGravatarSource()
	select {
	case <-time.After(timeout):
		err = fmt.Errorf("get gravatar image %s timeout", this.Hash)
	case err = <-thunder.GoFetch(gravatarSource+this.Hash+"?"+this.reqParams,
		this.imagePath):
	}
	return err
}

type service struct {
	cacheDir string
	altImage string
}

func (this *service) mustInt(r *http.Request, defaultValue int, keys ...string) (v int) {
	for _, k := range keys {
		if _, err := fmt.Sscanf(r.FormValue(k), "%d", &v); err == nil {
			defaultValue = v
		}
	}
	return defaultValue
}

func (this *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	hash := urlPath[strings.LastIndex(urlPath, "/")+1:]
	size := this.mustInt(r, 80, "s", "size") // default size = 80*80

	avatar := New(hash, this.cacheDir)
	avatar.AlterImage = this.altImage
	if avatar.Expired() {
		if err := avatar.UpdateTimeout(time.Millisecond * 1000); err != nil {
			log.Trace("avatar update error: %v", err)
			return
		}
	}
	if modtime, err := avatar.Modtime(); err == nil {
		etag := fmt.Sprintf("size(%d)", size)
		if t, err := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since")); err == nil && modtime.Before(t.Add(1*time.Second)) && etag == r.Header.Get("If-None-Match") {
			h := w.Header()
			delete(h, "Content-Type")
			delete(h, "Content-Length")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", etag)
	}
	w.Header().Set("Content-Type", "image/jpeg")

	if err := avatar.Encode(w, size); err != nil {
		log.Warn("avatar encode error: %v", err)
		w.WriteHeader(500)
	}
}

// http.Handle("/avatar/", avatar.CacheServer("./cache"))
func CacheServer(cacheDir string, defaultImgPath string) http.Handler {
	return &service{
		cacheDir: cacheDir,
		altImage: defaultImgPath,
	}
}

// thunder downloader
var thunder = &Thunder{QueueSize: 10}

type Thunder struct {
	QueueSize int // download queue size
	q         chan *thunderTask
	once      sync.Once
}

func (t *Thunder) init() {
	if t.QueueSize < 1 {
		t.QueueSize = 1
	}
	t.q = make(chan *thunderTask, t.QueueSize)
	for i := 0; i < t.QueueSize; i++ {
		go func() {
			for {
				task := <-t.q
				task.Fetch()
			}
		}()
	}
}

func (t *Thunder) Fetch(url string, saveFile string) error {
	t.once.Do(t.init)
	task := &thunderTask{
		Url:      url,
		SaveFile: saveFile,
	}
	task.Add(1)
	t.q <- task
	task.Wait()
	return task.err
}

func (t *Thunder) GoFetch(url, saveFile string) chan error {
	c := make(chan error)
	go func() {
		c <- t.Fetch(url, saveFile)
	}()
	return c
}

// thunder download
type thunderTask struct {
	Url      string
	SaveFile string
	sync.WaitGroup
	err error
}

func (this *thunderTask) Fetch() {
	this.err = this.fetch()
	this.Done()
}

var client = &http.Client{}

func (this *thunderTask) fetch() error {
	log.Debug("avatar.fetch(fetch new avatar): %s", this.Url)
	req, _ := http.NewRequest("GET", this.Url, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/jpeg,image/png,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "deflate,sdch")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/33.0.1750.154 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}

	/*
		log.Println("headers:", resp.Header)
		switch resp.Header.Get("Content-Type") {
		case "image/jpeg":
			this.SaveFile += ".jpeg"
		case "image/png":
			this.SaveFile += ".png"
		}
	*/
	/*
		imgType := resp.Header.Get("Content-Type")
		if imgType != "image/jpeg" && imgType != "image/png" {
			return errors.New("not png or jpeg")
		}
	*/

	tmpFile := this.SaveFile + ".part" // mv to destination when finished
	fd, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	_, err = io.Copy(fd, resp.Body)
	fd.Close()
	if err != nil {
		os.Remove(tmpFile)
		return err
	}
	return os.Rename(tmpFile, this.SaveFile)
}

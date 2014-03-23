package avatar

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var gravatar = "http://www.gravatar.com/avatar"

// hash email to md5 string
func HashEmail(email string) string {
	h := md5.New()
	h.Write([]byte(strings.ToLower(email)))
	return hex.EncodeToString(h.Sum(nil))
}

type Avatar struct {
	Hash      string
	cacheDir  string // image save dir
	reqParams string
	imagePath string
}

func New(hash string, cacheDir string) *Avatar {
	return &Avatar{
		Hash:     hash,
		cacheDir: cacheDir,
		reqParams: url.Values{
			"d":    {"retro"},
			"size": {"200"},
			"r":    {"pg"}}.Encode(),
		imagePath: filepath.Join(cacheDir, hash+".jpg"),
	}
}

// get image from gravatar.com
func (this *Avatar) Update() {
	thunder.Fetch(gravatar+"/"+this.Hash+"?"+this.reqParams,
		this.Hash+".jpg")
}

func (this *Avatar) UpdateTimeout(timeout time.Duration) {
	select {
	case <-time.After(timeout):
		log.Println("timeout")
	case <-thunder.GoFetch(gravatar+"/"+this.Hash+"?"+this.reqParams,
		this.Hash+".jpg"):
	}
}

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

func (this *thunderTask) fetch() error {
	resp, err := http.Get(this.Url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}
	fd, err := os.Create(this.SaveFile)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = io.Copy(fd, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

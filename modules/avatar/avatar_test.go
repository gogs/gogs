package avatar

import (
	"log"
	"strconv"
	"testing"
	"time"
)

func TestFetch(t *testing.T) {
	hash := HashEmail("ssx205@gmail.com")
	avatar := New(hash, "./")
	//avatar.Update()
	avatar.UpdateTimeout(time.Millisecond * 200)
	time.Sleep(5 * time.Second)
}

func TestFetchMany(t *testing.T) {
	log.Println("start")
	var n = 50
	ch := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			hash := HashEmail(strconv.Itoa(i) + "ssx205@gmail.com")
			avatar := New(hash, "./")
			avatar.Update()
			log.Println("finish", hash)
			ch <- true
		}(i)
	}
	for i := 0; i < n; i++ {
		<-ch
	}
	log.Println("end")
}

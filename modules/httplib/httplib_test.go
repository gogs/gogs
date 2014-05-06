package httplib

import (
	"io/ioutil"
	"testing"
)

func TestGetUrl(t *testing.T) {
	resp, err := Get("http://beego.me/").Debug(true).Response()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Body == nil {
		t.Fatal("body is nil")
	}
	data, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("data is no")
	}

	str, err := Get("http://beego.me/").String()
	if err != nil {
		t.Fatal(err)
	}
	if len(str) == 0 {
		t.Fatal("has no info")
	}
}

// Copyright 2013 The Beego Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package httplib

// NOTE: last sync c07b1d8 on Aug 23, 2014.

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var defaultSetting = BeegoHttpSettings{false, "beegoServer", 60 * time.Second, 60 * time.Second, nil, nil, nil, false}
var defaultCookieJar http.CookieJar
var settingMutex sync.Mutex

// createDefaultCookie creates a global cookiejar to store cookies.
func createDefaultCookie() {
	settingMutex.Lock()
	defer settingMutex.Unlock()
	defaultCookieJar, _ = cookiejar.New(nil)
}

// Overwrite default settings
func SetDefaultSetting(setting BeegoHttpSettings) {
	settingMutex.Lock()
	defer settingMutex.Unlock()
	defaultSetting = setting
	if defaultSetting.ConnectTimeout == 0 {
		defaultSetting.ConnectTimeout = 60 * time.Second
	}
	if defaultSetting.ReadWriteTimeout == 0 {
		defaultSetting.ReadWriteTimeout = 60 * time.Second
	}
}

// return *BeegoHttpRequest with specific method
func newBeegoRequest(url, method string) *BeegoHttpRequest {
	var resp http.Response
	req := http.Request{
		Method:     method,
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	return &BeegoHttpRequest{url, &req, map[string]string{}, map[string]string{}, defaultSetting, &resp, nil}
}

// Get returns *BeegoHttpRequest with GET method.
func Get(url string) *BeegoHttpRequest {
	return newBeegoRequest(url, "GET")
}

// Post returns *BeegoHttpRequest with POST method.
func Post(url string) *BeegoHttpRequest {
	return newBeegoRequest(url, "POST")
}

// Put returns *BeegoHttpRequest with PUT method.
func Put(url string) *BeegoHttpRequest {
	return newBeegoRequest(url, "PUT")
}

// Delete returns *BeegoHttpRequest DELETE method.
func Delete(url string) *BeegoHttpRequest {
	return newBeegoRequest(url, "DELETE")
}

// Head returns *BeegoHttpRequest with HEAD method.
func Head(url string) *BeegoHttpRequest {
	return newBeegoRequest(url, "HEAD")
}

// BeegoHttpSettings
type BeegoHttpSettings struct {
	ShowDebug        bool
	UserAgent        string
	ConnectTimeout   time.Duration
	ReadWriteTimeout time.Duration
	TlsClientConfig  *tls.Config
	Proxy            func(*http.Request) (*url.URL, error)
	Transport        http.RoundTripper
	EnableCookie     bool
}

// BeegoHttpRequest provides more useful methods for requesting one url than http.Request.
type BeegoHttpRequest struct {
	url     string
	req     *http.Request
	params  map[string]string
	files   map[string]string
	setting BeegoHttpSettings
	resp    *http.Response
	body    []byte
}

// Change request settings
func (b *BeegoHttpRequest) Setting(setting BeegoHttpSettings) *BeegoHttpRequest {
	b.setting = setting
	return b
}

// SetBasicAuth sets the request's Authorization header to use HTTP Basic Authentication with the provided username and password.
func (b *BeegoHttpRequest) SetBasicAuth(username, password string) *BeegoHttpRequest {
	b.req.SetBasicAuth(username, password)
	return b
}

// SetEnableCookie sets enable/disable cookiejar
func (b *BeegoHttpRequest) SetEnableCookie(enable bool) *BeegoHttpRequest {
	b.setting.EnableCookie = enable
	return b
}

// SetUserAgent sets User-Agent header field
func (b *BeegoHttpRequest) SetUserAgent(useragent string) *BeegoHttpRequest {
	b.setting.UserAgent = useragent
	return b
}

// Debug sets show debug or not when executing request.
func (b *BeegoHttpRequest) Debug(isdebug bool) *BeegoHttpRequest {
	b.setting.ShowDebug = isdebug
	return b
}

// SetTimeout sets connect time out and read-write time out for BeegoRequest.
func (b *BeegoHttpRequest) SetTimeout(connectTimeout, readWriteTimeout time.Duration) *BeegoHttpRequest {
	b.setting.ConnectTimeout = connectTimeout
	b.setting.ReadWriteTimeout = readWriteTimeout
	return b
}

// SetTLSClientConfig sets tls connection configurations if visiting https url.
func (b *BeegoHttpRequest) SetTLSClientConfig(config *tls.Config) *BeegoHttpRequest {
	b.setting.TlsClientConfig = config
	return b
}

// Header add header item string in request.
func (b *BeegoHttpRequest) Header(key, value string) *BeegoHttpRequest {
	b.req.Header.Set(key, value)
	return b
}

// Set the protocol version for incoming requests.
// Client requests always use HTTP/1.1.
func (b *BeegoHttpRequest) SetProtocolVersion(vers string) *BeegoHttpRequest {
	if len(vers) == 0 {
		vers = "HTTP/1.1"
	}

	major, minor, ok := http.ParseHTTPVersion(vers)
	if ok {
		b.req.Proto = vers
		b.req.ProtoMajor = major
		b.req.ProtoMinor = minor
	}

	return b
}

// SetCookie add cookie into request.
func (b *BeegoHttpRequest) SetCookie(cookie *http.Cookie) *BeegoHttpRequest {
	b.req.Header.Add("Cookie", cookie.String())
	return b
}

// Set transport to
func (b *BeegoHttpRequest) SetTransport(transport http.RoundTripper) *BeegoHttpRequest {
	b.setting.Transport = transport
	return b
}

// Set http proxy
// example:
//
//	func(req *http.Request) (*url.URL, error) {
// 		u, _ := url.ParseRequestURI("http://127.0.0.1:8118")
// 		return u, nil
// 	}
func (b *BeegoHttpRequest) SetProxy(proxy func(*http.Request) (*url.URL, error)) *BeegoHttpRequest {
	b.setting.Proxy = proxy
	return b
}

// Param adds query param in to request.
// params build query string as ?key1=value1&key2=value2...
func (b *BeegoHttpRequest) Param(key, value string) *BeegoHttpRequest {
	b.params[key] = value
	return b
}

func (b *BeegoHttpRequest) PostFile(formname, filename string) *BeegoHttpRequest {
	b.files[formname] = filename
	return b
}

// Body adds request raw body.
// it supports string and []byte.
func (b *BeegoHttpRequest) Body(data interface{}) *BeegoHttpRequest {
	switch t := data.(type) {
	case string:
		bf := bytes.NewBufferString(t)
		b.req.Body = ioutil.NopCloser(bf)
		b.req.ContentLength = int64(len(t))
	case []byte:
		bf := bytes.NewBuffer(t)
		b.req.Body = ioutil.NopCloser(bf)
		b.req.ContentLength = int64(len(t))
	}
	return b
}

func (b *BeegoHttpRequest) getResponse() (*http.Response, error) {
	if b.resp.StatusCode != 0 {
		return b.resp, nil
	}
	var paramBody string
	if len(b.params) > 0 {
		var buf bytes.Buffer
		for k, v := range b.params {
			buf.WriteString(url.QueryEscape(k))
			buf.WriteByte('=')
			buf.WriteString(url.QueryEscape(v))
			buf.WriteByte('&')
		}
		paramBody = buf.String()
		paramBody = paramBody[0 : len(paramBody)-1]
	}

	if b.req.Method == "GET" && len(paramBody) > 0 {
		if strings.Index(b.url, "?") != -1 {
			b.url += "&" + paramBody
		} else {
			b.url = b.url + "?" + paramBody
		}
	} else if b.req.Method == "POST" && b.req.Body == nil && len(paramBody) > 0 {
		if len(b.files) > 0 {
			bodyBuf := &bytes.Buffer{}
			bodyWriter := multipart.NewWriter(bodyBuf)
			for formname, filename := range b.files {
				fileWriter, err := bodyWriter.CreateFormFile(formname, filename)
				if err != nil {
					return nil, err
				}
				fh, err := os.Open(filename)
				if err != nil {
					return nil, err
				}
				//iocopy
				_, err = io.Copy(fileWriter, fh)
				fh.Close()
				if err != nil {
					return nil, err
				}
			}
			for k, v := range b.params {
				bodyWriter.WriteField(k, v)
			}
			contentType := bodyWriter.FormDataContentType()
			bodyWriter.Close()
			b.Header("Content-Type", contentType)
			b.req.Body = ioutil.NopCloser(bodyBuf)
			b.req.ContentLength = int64(bodyBuf.Len())
		} else {
			b.Header("Content-Type", "application/x-www-form-urlencoded")
			b.Body(paramBody)
		}
	}

	url, err := url.Parse(b.url)
	if err != nil {
		return nil, err
	}

	b.req.URL = url

	trans := b.setting.Transport

	if trans == nil {
		// create default transport
		trans = &http.Transport{
			TLSClientConfig: b.setting.TlsClientConfig,
			Proxy:           b.setting.Proxy,
			Dial:            TimeoutDialer(b.setting.ConnectTimeout, b.setting.ReadWriteTimeout),
		}
	} else {
		// if b.transport is *http.Transport then set the settings.
		if t, ok := trans.(*http.Transport); ok {
			if t.TLSClientConfig == nil {
				t.TLSClientConfig = b.setting.TlsClientConfig
			}
			if t.Proxy == nil {
				t.Proxy = b.setting.Proxy
			}
			if t.Dial == nil {
				t.Dial = TimeoutDialer(b.setting.ConnectTimeout, b.setting.ReadWriteTimeout)
			}
		}
	}

	var jar http.CookieJar
	if b.setting.EnableCookie {
		if defaultCookieJar == nil {
			createDefaultCookie()
		}
		jar = defaultCookieJar
	} else {
		jar = nil
	}

	client := &http.Client{
		Transport: trans,
		Jar:       jar,
	}

	if b.setting.UserAgent != "" {
		b.req.Header.Set("User-Agent", b.setting.UserAgent)
	}

	if b.setting.ShowDebug {
		dump, err := httputil.DumpRequest(b.req, true)
		if err != nil {
			println(err.Error())
		}
		println(string(dump))
	}

	resp, err := client.Do(b.req)
	if err != nil {
		return nil, err
	}
	b.resp = resp
	return resp, nil
}

// String returns the body string in response.
// it calls Response inner.
func (b *BeegoHttpRequest) String() (string, error) {
	data, err := b.Bytes()
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Bytes returns the body []byte in response.
// it calls Response inner.
func (b *BeegoHttpRequest) Bytes() ([]byte, error) {
	if b.body != nil {
		return b.body, nil
	}
	resp, err := b.getResponse()
	if err != nil {
		return nil, err
	}
	if resp.Body == nil {
		return nil, nil
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	b.body = data
	return data, nil
}

// ToFile saves the body data in response to one file.
// it calls Response inner.
func (b *BeegoHttpRequest) ToFile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	resp, err := b.getResponse()
	if err != nil {
		return err
	}
	if resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// ToJson returns the map that marshals from the body bytes as json in response .
// it calls Response inner.
func (b *BeegoHttpRequest) ToJson(v interface{}) error {
	data, err := b.Bytes()
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, v)
	return err
}

// ToXml returns the map that marshals from the body bytes as xml in response .
// it calls Response inner.
func (b *BeegoHttpRequest) ToXml(v interface{}) error {
	data, err := b.Bytes()
	if err != nil {
		return err
	}
	err = xml.Unmarshal(data, v)
	return err
}

// Response executes request client gets response mannually.
func (b *BeegoHttpRequest) Response() (*http.Response, error) {
	return b.getResponse()
}

// TimeoutDialer returns functions of connection dialer with timeout settings for http.Transport Dial field.
func TimeoutDialer(cTimeout time.Duration, rwTimeout time.Duration) func(net, addr string) (c net.Conn, err error) {
	return func(netw, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(netw, addr, cTimeout)
		if err != nil {
			return nil, err
		}
		conn.SetDeadline(time.Now().Add(rwTimeout))
		return conn, nil
	}
}

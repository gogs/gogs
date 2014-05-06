// Copyright 2013 The Beego Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package httplib

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

var defaultUserAgent = "gogsServer"

// Get returns *BeegoHttpRequest with GET method.
func Get(url string) *BeegoHttpRequest {
	var req http.Request
	req.Method = "GET"
	req.Header = http.Header{}
	req.Header.Set("User-Agent", defaultUserAgent)
	return &BeegoHttpRequest{url, &req, map[string]string{}, false, 60 * time.Second, 60 * time.Second, nil, nil, nil}
}

// Post returns *BeegoHttpRequest with POST method.
func Post(url string) *BeegoHttpRequest {
	var req http.Request
	req.Method = "POST"
	req.Header = http.Header{}
	req.Header.Set("User-Agent", defaultUserAgent)
	return &BeegoHttpRequest{url, &req, map[string]string{}, false, 60 * time.Second, 60 * time.Second, nil, nil, nil}
}

// Put returns *BeegoHttpRequest with PUT method.
func Put(url string) *BeegoHttpRequest {
	var req http.Request
	req.Method = "PUT"
	req.Header = http.Header{}
	req.Header.Set("User-Agent", defaultUserAgent)
	return &BeegoHttpRequest{url, &req, map[string]string{}, false, 60 * time.Second, 60 * time.Second, nil, nil, nil}
}

// Delete returns *BeegoHttpRequest DELETE GET method.
func Delete(url string) *BeegoHttpRequest {
	var req http.Request
	req.Method = "DELETE"
	req.Header = http.Header{}
	req.Header.Set("User-Agent", defaultUserAgent)
	return &BeegoHttpRequest{url, &req, map[string]string{}, false, 60 * time.Second, 60 * time.Second, nil, nil, nil}
}

// Head returns *BeegoHttpRequest with HEAD method.
func Head(url string) *BeegoHttpRequest {
	var req http.Request
	req.Method = "HEAD"
	req.Header = http.Header{}
	req.Header.Set("User-Agent", defaultUserAgent)
	return &BeegoHttpRequest{url, &req, map[string]string{}, false, 60 * time.Second, 60 * time.Second, nil, nil, nil}
}

// BeegoHttpRequest provides more useful methods for requesting one url than http.Request.
type BeegoHttpRequest struct {
	url              string
	req              *http.Request
	params           map[string]string
	showdebug        bool
	connectTimeout   time.Duration
	readWriteTimeout time.Duration
	tlsClientConfig  *tls.Config
	proxy            func(*http.Request) (*url.URL, error)
	transport        http.RoundTripper
}

// Debug sets show debug or not when executing request.
func (b *BeegoHttpRequest) Debug(isdebug bool) *BeegoHttpRequest {
	b.showdebug = isdebug
	return b
}

// SetTimeout sets connect time out and read-write time out for BeegoRequest.
func (b *BeegoHttpRequest) SetTimeout(connectTimeout, readWriteTimeout time.Duration) *BeegoHttpRequest {
	b.connectTimeout = connectTimeout
	b.readWriteTimeout = readWriteTimeout
	return b
}

// SetTLSClientConfig sets tls connection configurations if visiting https url.
func (b *BeegoHttpRequest) SetTLSClientConfig(config *tls.Config) *BeegoHttpRequest {
	b.tlsClientConfig = config
	return b
}

// Header add header item string in request.
func (b *BeegoHttpRequest) Header(key, value string) *BeegoHttpRequest {
	b.req.Header.Set(key, value)
	return b
}

// SetCookie add cookie into request.
func (b *BeegoHttpRequest) SetCookie(cookie *http.Cookie) *BeegoHttpRequest {
	b.req.Header.Add("Cookie", cookie.String())
	return b
}

// Set transport to
func (b *BeegoHttpRequest) SetTransport(transport http.RoundTripper) *BeegoHttpRequest {
	b.transport = transport
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
	b.proxy = proxy
	return b
}

// Param adds query param in to request.
// params build query string as ?key1=value1&key2=value2...
func (b *BeegoHttpRequest) Param(key, value string) *BeegoHttpRequest {
	b.params[key] = value
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
		b.Header("Content-Type", "application/x-www-form-urlencoded")
		b.Body(paramBody)
	}

	url, err := url.Parse(b.url)
	if url.Scheme == "" {
		b.url = "http://" + b.url
		url, err = url.Parse(b.url)
	}
	if err != nil {
		return nil, err
	}

	b.req.URL = url
	if b.showdebug {
		dump, err := httputil.DumpRequest(b.req, true)
		if err != nil {
			println(err.Error())
		}
		println(string(dump))
	}

	trans := b.transport

	if trans == nil {
		// create default transport
		trans = &http.Transport{
			TLSClientConfig: b.tlsClientConfig,
			Proxy:           b.proxy,
			Dial:            TimeoutDialer(b.connectTimeout, b.readWriteTimeout),
		}
	} else {
		// if b.transport is *http.Transport then set the settings.
		if t, ok := trans.(*http.Transport); ok {
			if t.TLSClientConfig == nil {
				t.TLSClientConfig = b.tlsClientConfig
			}
			if t.Proxy == nil {
				t.Proxy = b.proxy
			}
			if t.Dial == nil {
				t.Dial = TimeoutDialer(b.connectTimeout, b.readWriteTimeout)
			}
		}
	}

	client := &http.Client{
		Transport: trans,
	}

	resp, err := client.Do(b.req)
	if err != nil {
		return nil, err
	}
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
	if err != nil {
		return err
	}
	return nil
}

// ToJson returns the map that marshals from the body bytes as json in response .
// it calls Response inner.
func (b *BeegoHttpRequest) ToJson(v interface{}) error {
	data, err := b.Bytes()
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}

// ToXml returns the map that marshals from the body bytes as xml in response .
// it calls Response inner.
func (b *BeegoHttpRequest) ToXML(v interface{}) error {
	data, err := b.Bytes()
	if err != nil {
		return err
	}
	err = xml.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
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

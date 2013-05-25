// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

var userAgent = "go application"

var (
	dialTimeout  = flag.Duration("dial_timeout", 10*time.Second, "Timeout for dialing an HTTP connection.")
	readTimeout  = flag.Duration("read_timeout", 10*time.Second, "Timeoout for reading an HTTP response.")
	writeTimeout = flag.Duration("write_timeout", 5*time.Second, "Timeout writing an HTTP request.")
)

type timeoutConn struct {
	net.Conn
}

func (c *timeoutConn) Read(p []byte) (int, error) {
	return c.Conn.Read(p)
}

func (c *timeoutConn) Write(p []byte) (int, error) {
	// Reset timeouts when writing a request.
	c.Conn.SetWriteDeadline(time.Now().Add(*readTimeout))
	c.Conn.SetWriteDeadline(time.Now().Add(*writeTimeout))
	return c.Conn.Write(p)
}
func timeoutDial(network, addr string) (net.Conn, error) {
	c, err := net.DialTimeout(network, addr, *dialTimeout)
	if err != nil {
		return nil, err
	}
	return &timeoutConn{Conn: c}, nil
}

var (
	httpTransport = &http.Transport{Dial: timeoutDial}
	HttpClient    = &http.Client{Transport: httpTransport}
)

// HttpGetBytes returns page data in []byte.
func HttpGetBytes(client *http.Client, url string, header http.Header) ([]byte, error) {
	rc, err := httpGet(client, url, header)
	if err != nil {
		return nil, err
	}
	p, err := ioutil.ReadAll(rc)
	rc.Close()
	return p, err
}

// httpGet gets the specified resource. ErrNotFound is returned if the
// server responds with status 404.
func httpGet(client *http.Client, url string, header http.Header) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, vs := range header {
		req.Header[k] = vs
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &RemoteError{req.URL.Host, err}
	}

	if resp.StatusCode == 200 {
		return resp.Body, nil
	}
	resp.Body.Close()
	if resp.StatusCode == 404 { // 403 can be rate limit error.  || resp.StatusCode == 403 {
		err = NotFoundError{"Resource not found: " + url}
	} else {
		err = &RemoteError{req.URL.Host, fmt.Errorf("get %s -> %d", url, resp.StatusCode)}
	}
	return nil, err
}

// fetchFiles fetches the source files specified by the rawURL field in parallel.
func fetchFiles(client *http.Client, files []*source, header http.Header) error {
	ch := make(chan error, len(files))
	for i := range files {
		go func(i int) {
			req, err := http.NewRequest("GET", files[i].rawURL, nil)
			if err != nil {
				ch <- err
				return
			}
			req.Header.Set("User-Agent", userAgent)
			for k, vs := range header {
				req.Header[k] = vs
			}
			resp, err := client.Do(req)
			if err != nil {
				ch <- &RemoteError{req.URL.Host, err}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				ch <- &RemoteError{req.URL.Host, fmt.Errorf("get %s -> %d", req.URL, resp.StatusCode)}
				return
			}
			files[i].data, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				ch <- &RemoteError{req.URL.Host, err}
				return
			}
			ch <- nil
		}(i)
	}
	for _ = range files {
		if err := <-ch; err != nil {
			return err
		}
	}
	return nil
}

func httpGetJSON(client *http.Client, url string, v interface{}) error {
	rc, err := httpGet(client, url, nil)
	if err != nil {
		return err
	}
	defer rc.Close()
	err = json.NewDecoder(rc).Decode(v)
	if _, ok := err.(*json.SyntaxError); ok {
		err = NotFoundError{"JSON syntax error at " + url}
	}
	return err
}

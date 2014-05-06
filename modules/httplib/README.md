# httplib
httplib is an libs help you to curl remote url.

# How to use?

## GET
you can use Get to crawl data.

	import "httplib"
	
	str, err := httplib.Get("http://beego.me/").String()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(str)
	
## POST
POST data to remote url

	b:=httplib.Post("http://beego.me/")
	b.Param("username","astaxie")
	b.Param("password","123456")
	str, err := b.String()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(str)

## set timeout
you can set timeout in request.default is 60 seconds.

set Get timeout:

	httplib.Get("http://beego.me/").SetTimeout(100 * time.Second, 30 * time.Second)
	
set post timeout:	
	
	httplib.Post("http://beego.me/").SetTimeout(100 * time.Second, 30 * time.Second)

- first param is connectTimeout.
- second param is readWriteTimeout

## debug
if you want to debug the request info, set the debug on

	httplib.Get("http://beego.me/").Debug(true)
	
## support HTTPS client
if request url is https. You can set the client support TSL:

	httplib.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	
more info about the tls.Config please visit http://golang.org/pkg/crypto/tls/#Config	
		
## set cookie
some http request need setcookie. So set it like this:

	cookie := &http.Cookie{}
	cookie.Name = "username"
	cookie.Value  = "astaxie"
	httplib.Get("http://beego.me/").SetCookie(cookie)


package testutil

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gogits/gogs/cmd"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/routers"

	"gopkg.in/macaron.v1"
	"gopkg.in/testfixtures.v1"
)

const (
	CONTENT_TYPE_FORM = "application/x-www-form-urlencoded"
	CONTENT_TYPE_JSON = "application/json"
)

var (
	theMacaron *macaron.Macaron
)

func getMacaron() *macaron.Macaron {
	if theMacaron == nil {
		theMacaron = cmd.NewMacaron()
	}
	return theMacaron
}

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	getMacaron().ServeHTTP(w, r)
}

func NewTestContext(method, path, contentType string, body []byte, userId string) (w *httptest.ResponseRecorder, r *http.Request) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	w = httptest.NewRecorder()
	r, _ = http.NewRequest(method, path, bodyReader)
	if len(contentType) > 0 {
		r.Header.Set("Content-Type", contentType)
		if contentType == CONTENT_TYPE_FORM {
			r.PostForm = url.Values{}
		}
	}
	if len(userId) > 0 {
		r.AddCookie(&http.Cookie{Name: "user_id", Value: userId})
	}
	return
}

func TableCount(tableName string) (count int64) {
	models.Database().QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName)).Scan(&count)
	return
}

func LastId(tableName string) (lastId int64) {
	models.Database().QueryRow(fmt.Sprintf("SELECT MAX(id) FROM \"%s\"", tableName)).Scan(&lastId)
	return
}

func TestGlobalInit() {
	setting.CustomConf = setting.GogsPath() + "testdata/app_test.ini"
	routers.GlobalInit()
}

func PrepareTestDatabase() {
	err := testfixtures.LoadFixtures(setting.GogsPath()+"testdata/fixtures", models.Database(), &testfixtures.SQLiteHelper{})
	if err != nil {
		log.Fatalf("Error while loading fixtures to database: %v", err)
	}
}

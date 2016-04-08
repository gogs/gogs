package repo_test

import (
	"encoding/json"
	"net/http"
	"testing"

	api "github.com/gogits/go-gogs-client"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIssuesAPI(t *testing.T) {
	Convey("Given the issue API", t, func() {
		testutil.TestGlobalInit()

		Convey("It should return issues from the index", func() {
			testutil.PrepareTestDatabase()

			w, r := testutil.NewTestContext("GET", "/api/v1/repos/user1/foo/issues", "", nil, "1")
			testutil.ServeHTTP(w, r)
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("It should show a specific issue", func() {
			testutil.PrepareTestDatabase()

			w, r := testutil.NewTestContext("GET", "/api/v1/repos/user1/foo/issues/1", "", nil, "1")
			testutil.ServeHTTP(w, r)
			So(w.Code, ShouldEqual, http.StatusOK)

			var issue api.Issue
			json.Unmarshal(w.Body.Bytes(), &issue)
			So(issue.Title, ShouldEqual, "Title")
			So(issue.Body, ShouldEqual, "Content")
			So(issue.User.UserName, ShouldEqual, "user1")
		})

		Convey("It should create issue", func() {
			testutil.PrepareTestDatabase()

			bytes, _ := json.Marshal(api.Issue{
				Title: "A issue title",
				Body:  "Please fix",
			})
			count := testutil.TableCount("issue")
			w, r := testutil.NewTestContext("POST", "/api/v1/repos/user1/foo/issues", testutil.CONTENT_TYPE_JSON, bytes, "1")
			testutil.ServeHTTP(w, r)
			So(w.Code, ShouldEqual, http.StatusCreated)
			So(testutil.TableCount("issue"), ShouldEqual, count+1)
			issue, _ := models.GetIssueByID(testutil.LastId("issue"))
			So(issue.Name, ShouldEqual, "A issue title")
			So(issue.Content, ShouldEqual, "Please fix")
		})

		Convey("It should edit issue", func() {
			testutil.PrepareTestDatabase()

			bytes, _ := json.Marshal(api.Issue{
				Title: "Edited title",
				Body:  "Edited content",
			})
			w, r := testutil.NewTestContext("PATCH", "/api/v1/repos/user1/foo/issues/1", testutil.CONTENT_TYPE_JSON, bytes, "1")
			testutil.ServeHTTP(w, r)
			So(w.Code, ShouldEqual, http.StatusCreated)
			issue, _ := models.GetIssueByID(1)
			So(issue.Name, ShouldEqual, "Edited title")
			So(issue.Content, ShouldEqual, "Edited content")
		})
	})
}

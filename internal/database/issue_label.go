package database

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/lazyregexp"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

var labelColorPattern = lazyregexp.New("#([a-fA-F0-9]{6})")

// GetLabelTemplateFile loads the label template file by given name,
// then parses and returns a list of name-color pairs.
func GetLabelTemplateFile(name string) ([][2]string, error) {
	data, err := getRepoInitFile("label", name)
	if err != nil {
		return nil, errors.Newf("getRepoInitFile: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	list := make([][2]string, 0, len(lines))
	for i := range lines {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			return nil, errors.Newf("line is malformed: %s", line)
		}

		if !labelColorPattern.MatchString(fields[0]) {
			return nil, errors.Newf("bad HTML color code in line: %s", line)
		}

		fields[1] = strings.TrimSpace(fields[1])
		list = append(list, [2]string{fields[1], fields[0]})
	}

	return list, nil
}

// Label represents a label of repository for issues.
type Label struct {
	ID              int64
	RepoID          int64 `xorm:"INDEX"`
	Name            string
	Color           string `xorm:"VARCHAR(7)"`
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `xorm:"-" json:"-" gorm:"-"`
	IsChecked       bool `xorm:"-" json:"-" gorm:"-"`
}

func (l *Label) APIFormat() *apiv1types.IssueLabel {
	return &apiv1types.IssueLabel{
		ID:    l.ID,
		Name:  l.Name,
		Color: strings.TrimLeft(l.Color, "#"),
	}
}

// CalOpenIssues calculates the open issues of label.
func (l *Label) CalOpenIssues() {
	l.NumOpenIssues = l.NumIssues - l.NumClosedIssues
}

// ForegroundColor calculates the text color for labels based
// on their background color.
func (l *Label) ForegroundColor() template.CSS {
	if strings.HasPrefix(l.Color, "#") {
		if color, err := strconv.ParseUint(l.Color[1:], 16, 64); err == nil {
			r := float32(0xFF & (color >> 16))
			g := float32(0xFF & (color >> 8))
			b := float32(0xFF & color)
			luminance := (0.2126*r + 0.7152*g + 0.0722*b) / 255

			if luminance < 0.66 {
				return template.CSS("#fff")
			}
		}
	}

	// default to black
	return template.CSS("#000")
}

var _ errx.NotFound = (*ErrLabelNotExist)(nil)

type ErrLabelNotExist struct {
	args map[string]any
}

func IsErrLabelNotExist(err error) bool {
	_, ok := err.(ErrLabelNotExist)
	return ok
}

func (err ErrLabelNotExist) Error() string {
	return fmt.Sprintf("label does not exist: %v", err.args)
}

func (ErrLabelNotExist) NotFound() bool {
	return true
}

// .___                            .____          ___.          .__
// |   | ______ ________ __   ____ |    |   _____ \_ |__   ____ |  |
// |   |/  ___//  ___/  |  \_/ __ \|    |   \__  \ | __ \_/ __ \|  |
// |   |\___ \ \___ \|  |  /\  ___/|    |___ / __ \| \_\ \  ___/|  |__
// |___/____  >____  >____/  \___  >_______ (____  /___  /\___  >____/
//          \/     \/            \/        \/    \/    \/     \/

// IssueLabel represents an issue-lable relation.
type IssueLabel struct {
	ID      int64
	IssueID int64 `gorm:"uniqueIndex:UQE_issue_label_s"`
	LabelID int64 `gorm:"uniqueIndex:UQE_issue_label_s"`
}

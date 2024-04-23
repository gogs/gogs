// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"sort"
	"time"

	"github.com/gogs/git-module"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/gitutil"
)

const (
	INSIGHT_CONTRIBUTORS   string = "repo/insights/contributors"
	INSIGHT_COMMITS        string = "repo/insights/commits"
	INSIGHT_CODE_FREQUENCY string = "repo/insights/code_frequency"
)

// ChartData represents the data structure for the chart.
type ChartData struct {
	Labels  []string `json:"labels"`
	Dataset struct {
		Label           string   `json:"label"`
		Data            []int    `json:"data"`
		BackgroundColor []string `json:"backgroundColor"`
		BorderColor     []string `json:"borderColor"`
		BorderWidth     int      `json:"borderWidth"`
	} `json:"dataset"`
}

// ContributorChartData represents the data structure for the contributors main data.
type ContributorChartData struct {
	Additions ChartData `json:"additions"`
	Deletions ChartData `json:"deletions"`
	Commits   ChartData `json:"commits"`
}

// commitData represents the data structure for the commit.
type commitData struct {
	Commit    *git.Commit
	Additions int
	Deletions int
}

// authorData represents the data structure for the author.
type authorData struct {
	*userCommit
	CommitsData    []*commitData
	ChartData      ChartData
	NumOfCommits   int
	NumOfAdditions int
	NumOfDeletions int
}

// CommitsChartData represents the data structure for the commits chart.
type CommitsChartData struct {
	CommitsByWeek  ChartData `json:"commits_by_week"`
	CommitsInAWeek ChartData `json:"commits_in_a_week"`
}

// contributionType represents the type of contribution.
type contributionType string

const (
	contributionTypeCommit   contributionType = "c"
	contributionTypeAddition contributionType = "a"
	contributionTypeDeletion contributionType = "d"
)

// InsightContributorsPage represents the GET method for the contributors insight page.
func InsightContributorsPage(c *context.Context) {
	c.Title("repo.insights.contributors")
	c.PageIs("InsightsContributors")

	// Get context query
	ctxQueryFrom := c.Query("from")
	ctxQueryTo := c.Query("to")
	ctxQueryType := c.Query("type")

	defaultBranch := c.Repo.Repository.DefaultBranch

	// Get commit data for the default branch
	commits, err := getCommitData(c, defaultBranch, true)
	if err != nil {
		c.Error(err, "get commits")
		return
	}

	// sort and filter commits
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Commit.Author.When.After(commits[j].Commit.Author.When)
	})

	// Get first and latest commit time
	firstCommitTime := commits[len(commits)-1].Commit.Author.When.Add(-24 * time.Hour)
	latestCommitTime := commits[0].Commit.Author.When.Add(24 * time.Hour)
	if ctxQueryFrom != "" && ctxQueryTo != "" {
		ctxQueryFromTime, _ := time.Parse(time.DateOnly, ctxQueryFrom)
		if firstCommitTime.Before(ctxQueryFromTime) {
			firstCommitTime = ctxQueryFromTime
		}
		ctxQueryToTime, _ := time.Parse(time.DateOnly, ctxQueryTo)
		if latestCommitTime.After(ctxQueryToTime) {
			latestCommitTime = ctxQueryToTime
		}
	}

	filteredCommits := make([]*commitData, 0)
	for _, commit := range commits {
		if !(commit.Commit.Author.When.Before(firstCommitTime) || commit.Commit.Author.When.After(latestCommitTime)) {
			filteredCommits = append(filteredCommits, commit)
		}
	}

	contributorChartData := getContributorChartData(filteredCommits, nil, nil)
	authorDataSlice := getContributorsAuthorData(c, filteredCommits, contributionType(ctxQueryType))

	c.Data["RangeStart"] = firstCommitTime
	c.Data["RangeEnd"] = latestCommitTime
	c.Data["RangeStartStr"] = firstCommitTime.Format(time.DateOnly)
	c.Data["RangeEndStr"] = latestCommitTime.Format(time.DateOnly)
	c.Data["DefaultBranch"] = defaultBranch
	switch contributionType(ctxQueryType) {
	case contributionTypeAddition:
		c.Data["ContributorsMainChartData"] = contributorChartData.Additions
	case contributionTypeDeletion:
		c.Data["ContributorsMainChartData"] = contributorChartData.Deletions
	default:
		c.Data["ContributorsMainChartData"] = contributorChartData.Commits
	}
	c.Data["Authors"] = authorDataSlice
	c.Data["RequireChartJS"] = true

	c.RequireAutosize()
	c.Success(INSIGHT_CONTRIBUTORS)
}

// InsightCommitsPage represents the GET method for the commits insight page.
func InsightCommitsPage(c *context.Context) {
	c.Title("repo.insights.commits")
	c.PageIs("InsightsCommits")

	// Get context query
	ctxQueryWeekID := c.Query("week_id") // e.g. 2019-52
	var yearID, weekID int
	fmt.Sscanf(ctxQueryWeekID, "%d-%d", &yearID, &weekID)

	// Get commit data for the default branch
	commits, err := getCommitData(c, c.Repo.Repository.DefaultBranch, false)
	if err != nil {
		c.Error(err, "get commits")
		return
	}

	// sort commits
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Commit.Author.When.Before(commits[j].Commit.Author.When)
	})

	commitChartData := getCommitChartData(commits, yearID, weekID)
	c.Data["CommitsByWeekChartData"] = commitChartData.CommitsByWeek
	c.Data["CommitsInAWeekChartData"] = commitChartData.CommitsInAWeek

	c.Data["RequireChartJS"] = true
	c.RequireAutosize()
	c.Success(INSIGHT_COMMITS)
}

// InsightCodeFrequencyPage represents the GET method for the code frequency insight page.
func InsightCodeFrequencyPage(c *context.Context) {
	c.Title("repo.insights.code_frequency")
	c.PageIs("InsightsCodeFrequency")

	c.Data["RequireChartJS"] = true
	c.RequireAutosize()
	c.Success(INSIGHT_CODE_FREQUENCY)
}

// InsightsGroup represents the handler for the insights group.
func InsightsGroup(c *context.Context) {
	c.PageIs("Insights")
}

// getContributorChartData returns the ContributorChartData struct that
// will be used in page's template. It takes a slice of *commitData and returns a
// ContributorChartData struct. The ContributorChartData struct contains
// three ChartData structs: Additions, Deletions, and Commits. Each ChartData struct
// represents a dataset for the chart, with labels and data.
//
// NOTE: The input commits slice is expected to be already sorted by commit time.
func getContributorChartData(commits []*commitData, rangeFrom, rangeTo *time.Time) ContributorChartData {
	commitsByDay := groupCommitsByDay(commits, rangeFrom, rangeTo)
	commitChartData := ChartData{}
	additionChartData := ChartData{}
	deletionChartData := ChartData{}
	date := commits[len(commits)-1].Commit.Author.When

	for _, commits := range commitsByDay {
		commitChartData.Labels = append(commitChartData.Labels, date.Format("2006-01-02"))
		additionChartData.Labels = append(additionChartData.Labels, date.Format("2006-01-02"))
		deletionChartData.Labels = append(deletionChartData.Labels, date.Format("2006-01-02"))
		totalDailyAddition := 0
		totalDailyDeletion := 0
		for _, commit := range commits {
			totalDailyAddition += commit.Additions
			totalDailyDeletion += commit.Deletions
		}
		date = date.Add(24 * time.Hour)
		commitChartData.Dataset.Data = append(commitChartData.Dataset.Data, len(commits))
		additionChartData.Dataset.Data = append(additionChartData.Dataset.Data, totalDailyAddition)
		deletionChartData.Dataset.Data = append(deletionChartData.Dataset.Data, totalDailyDeletion)
	}
	commitChartData.Dataset.Label = "Commits"
	additionChartData.Dataset.Label = "Additions"
	deletionChartData.Dataset.Label = "Deletions"

	return ContributorChartData{
		Additions: additionChartData,
		Deletions: deletionChartData,
		Commits:   commitChartData,
	}
}

// getContributorsAuthorData returns a slice of authorData structs. Each authorData
// struct contains the author's name, email, and the number of commits, additions,
// and deletions made by the author.
//
// NOTE: The input commits slice is expected to be already sorted by commit time.
func getContributorsAuthorData(ctx *context.Context, commitsData []*commitData, _contributiontype contributionType) []authorData {
	authorDataMap := make(map[string]authorData)

	firstCommitTime := commitsData[len(commitsData)-1].Commit.Author.When.Truncate(24 * time.Hour)
	latestCommitTime := commitsData[0].Commit.Author.When.Truncate(24 * time.Hour)

	commits := make([]*git.Commit, 0)
	for _, commitData := range commitsData {
		commits = append(commits, commitData.Commit)
	}
	userCommits := matchUsersWithCommitEmails(ctx.Req.Context(), commits)

	for i, userCommit := range userCommits {
		authorEmail := userCommit.Commit.Author.Email
		_authorData, ok := authorDataMap[authorEmail]
		if !ok {
			_authorData = authorData{}
			_authorData.userCommit = userCommit
			_authorData.CommitsData = make([]*commitData, 0)
		}
		_authorData.NumOfCommits++
		_authorData.NumOfAdditions += commitsData[i].Additions
		_authorData.NumOfDeletions += commitsData[i].Deletions
		_authorData.CommitsData = append(_authorData.CommitsData, commitsData[i])
		authorDataMap[authorEmail] = _authorData
	}

	authorDataSlice := make([]authorData, 0, len(authorDataMap))
	for _, authorData := range authorDataMap {
		authorDataSlice = append(authorDataSlice, authorData)
	}

	sort.Slice(authorDataSlice, func(i, j int) bool {
		switch _contributiontype {
		case contributionTypeAddition:
			return authorDataSlice[i].NumOfCommits > authorDataSlice[j].NumOfCommits
		case contributionTypeDeletion:
			return authorDataSlice[i].NumOfDeletions > authorDataSlice[j].NumOfDeletions
		default:
			return authorDataSlice[i].NumOfCommits > authorDataSlice[j].NumOfCommits
		}
	})

	for i, _authorData := range authorDataSlice {
		switch _contributiontype {
		case contributionTypeAddition:
			authorDataSlice[i].ChartData = getContributorChartData(_authorData.CommitsData, &firstCommitTime, &latestCommitTime).Additions
		case contributionTypeDeletion:
			authorDataSlice[i].ChartData = getContributorChartData(_authorData.CommitsData, &firstCommitTime, &latestCommitTime).Deletions
		default:
			authorDataSlice[i].ChartData = getContributorChartData(_authorData.CommitsData, &firstCommitTime, &latestCommitTime).Commits
		}
	}

	return authorDataSlice
}

// getCommitChartData returns the CommitsChartData struct that
// will be used in page's template. It takes a slice of *commitData and returns a
// CommitsChartData struct. The CommitsChartData struct contains
// two ChartData structs: CommitsByWeek and CommitsInAWeek. Each ChartData struct
// represents a dataset for the chart, with labels and data.
//
// NOTE: The input commits slice is expected to be already sorted by commit time.
func getCommitChartData(commits []*commitData, year, week int) CommitsChartData {
	commitsByWeek := groupCommitsByWeek(commits)
	commitsByWeekChartData := ChartData{Labels: []string{}}
	commitsInAWeekChartData := ChartData{Labels: []string{}}

	if len(commits) == 0 {
		return CommitsChartData{}
	}

	curWeekTime := commits[0].Commit.Author.When

	for _, _ = range commitsByWeek {
		curYear, curWeek := curWeekTime.ISOWeek()
		commitsByWeekChartData.Labels = append(commitsByWeekChartData.Labels, fmt.Sprintf("%04d-%02d", curYear, curWeek))
		curWeekTime = curWeekTime.Add(7 * 24 * time.Hour)
	}
	commitInAWeekData := make([]int, 7)
	for _, commit := range commits {
		curYear, curWeek := commit.Commit.Author.When.ISOWeek()
		if curYear == year && curWeek == week {
			commitInAWeekData[commit.Commit.Author.When.Weekday()]++
		}
	}
	commitsByWeekChartData.Dataset.Label = "Commits by week"
	commitsByWeekChartData.Dataset.Data = commitsByWeek
	commitsInAWeekChartData.Labels = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
	commitsInAWeekChartData.Dataset.Label = "Commits in a week"
	commitsInAWeekChartData.Dataset.Data = commitInAWeekData

	return CommitsChartData{
		CommitsByWeek:  commitsByWeekChartData,
		CommitsInAWeek: commitsInAWeekChartData,
	}
}

// getCommitData returns a slice of commitData structs. If isFetchAdditionDeletion
// is enabled, each commitData struct contains a *git.Commit and the number of
// additions and deletions made in that commit.
func getCommitData(c *context.Context, branch string, isFetchAdditionDeletion bool) ([]*commitData, error) {
	res := make([]*commitData, 0)

	commits, err := c.Repo.GitRepo.Log(branch)
	if err != nil {
		c.Error(err, "get commits")
		return nil, err
	}
	if len(commits) == 0 {
		c.Error(err, "no commits")
		return nil, err
	}

	for _, commit := range commits {
		startCommitID := commit.ID.String()
		if commit.ParentsCount() > 0 {
			startCommit, _ := commit.ParentID(0)
			startCommitID = startCommit.String()
		}
		endCommitID := commit.ID.String()

		diff := &gitutil.Diff{Diff: &git.Diff{}}
		if isFetchAdditionDeletion {
			diff, _ = gitutil.RepoDiff(c.Repo.GitRepo,
				endCommitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
				git.DiffOptions{Base: startCommitID, Timeout: time.Duration(conf.Git.Timeout.Diff) * time.Second},
			)
		}
		res = append(res, &commitData{
			Commit:    commit,
			Additions: diff.TotalAdditions(),
			Deletions: diff.TotalDeletions(),
		})
	}

	return res, nil
}

// groupCommitsByDay groups commits by the day they were made. It takes a slice of
// *commitData and returns a slice of slices of *commitData. Each inner slice
// represents a day and contains all commits made on that day. If no commits
// were made on a particular day, an empty slice is appended.
//
// Example:
// Input: []*commitData{commitA_day01, commitB_day01, commitC_day02, commitD_day04}
//
//	Output: [][]*commitData{
//	   []*commitData{commitA_day01, commitB_day01}, []*commitData{commitC_day02},
//	   []*commitData{}, []*commitData{commitD_day04}}
//
// NOTE: The input commits slice is expected to be already sorted by commit time.
func groupCommitsByDay(commits []*commitData, rangeFrom, rangeTo *time.Time) [][]*commitData {
	res := make([][]*commitData, 0)

	firstCommitTime := commits[len(commits)-1].Commit.Author.When.Truncate(24 * time.Hour)
	latestCommitTime := commits[0].Commit.Author.When.Truncate(24 * time.Hour)
	if rangeFrom != nil {
		if firstCommitTime.After(*rangeFrom) {
			firstCommitTime = rangeFrom.Truncate(24 * time.Hour)
		}
	}
	if rangeTo != nil {
		if latestCommitTime.Before(*rangeTo) {
			latestCommitTime = rangeTo.Truncate(24 * time.Hour)
		}
	}
	numOfDays := int(latestCommitTime.Sub(firstCommitTime)/(24*time.Hour)) + 1

	commitBucketMap := make(map[string][]*commitData)
	for _, commit := range commits {
		dateStr := commit.Commit.Author.When.Format("2006-01-02")
		commitBucketMap[dateStr] = append(commitBucketMap[dateStr], commit)
	}

	for i := 0; i < numOfDays; i++ {
		dateStr := firstCommitTime.Add(time.Duration(i) * 24 * time.Hour).Format("2006-01-02")
		if _, ok := commitBucketMap[dateStr]; ok {
			res = append(res, commitBucketMap[dateStr])
		} else {
			res = append(res, make([]*commitData, 0))
		}
	}

	return res
}

// groupCommitsByWeek groups commits by the week they were made. It takes a slice of
// *commitData and returns a slice of integers representing the number of commits
// made in each week.
//
// Example:
// Input: []*commitData{commitA_week01, commitB_week01, commitC_week02, commitD_week04}
//
//	Output: []int{2, 1, 0, 1}
//
// NOTE: The input commits slice is expected to be already sorted by commit time.
func groupCommitsByWeek(commits []*commitData) []int {
	res := make([]int, 0)

	if len(commits) == 0 {
		return nil
	}

	nCommit := 0
	curTime := commits[0].Commit.Author.When
	for _, commit := range commits {
		y, w := commit.Commit.Author.When.ISOWeek()
		curYear, curWeek := curTime.ISOWeek()
		if w != curWeek || y != curYear {
			res = append(res, nCommit)

			// add missing week
			nMissingWeek := 0
			if y == curYear && w-curWeek > 1 {
				nMissingWeek += (w - curWeek - 1)
			}
			if y > curYear {
				curLastWeekOfTheYear, _ := time.Date(curYear, time.December, 31, 0, 0, 0, 0, curTime.Location()).ISOWeek()
				firstWeekOfTheYear, _ := time.Date(y, time.January, 1, 0, 0, 0, 0, curTime.Location()).ISOWeek()
				nMissingWeek += (curLastWeekOfTheYear - curWeek) + (y - curYear - 1) + (w - firstWeekOfTheYear)
			}
			for i := 0; i < nMissingWeek; i++ {
				res = append(res, 0)
			}

			curTime = commit.Commit.Author.When
			nCommit = 0
		}
		nCommit++
	}
	res = append(res, nCommit)
	return res
}

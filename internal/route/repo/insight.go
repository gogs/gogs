// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
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

// ContributorsMainChartData represents the data structure for the contributors main data.
type ContributorsMainChartData struct {
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

// contributionType represents the type of contribution.
const (
	contributionTypeCommit   string = "c"
	contributionTypeAddition string = "a"
	contributionTypeDeletion string = "d"
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
	commits, err := getCommitData(c, defaultBranch)
	if err != nil {
		c.Error(err, "get commits")
		return
	}

	// Get first and latest commit time
	firstCommitTime := commits[len(commits)-1].Commit.Author.When
	latestCommitTime := commits[0].Commit.Author.When
	if ctxQueryFrom != "" && ctxQueryTo != "" {
		firstCommitTime, _ = time.Parse(time.DateOnly, ctxQueryFrom)
		latestCommitTime, _ = time.Parse(time.DateOnly, ctxQueryTo)
	}

	// sort and filter commits
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Commit.Author.When.After(commits[j].Commit.Author.When)
	})

	filteredCommits := make([]*commitData, 0)
	for _, commit := range commits {
		if commit.Commit.Author.When.After(firstCommitTime) && commit.Commit.Author.When.Before(latestCommitTime) {
			filteredCommits = append(filteredCommits, commit)
		}
	}

	contributorsMainChartData := getContributorsMainChartData(commits)

	c.Data["RangeStart"] = firstCommitTime
	c.Data["RangeEnd"] = latestCommitTime
	c.Data["RangeStartStr"] = firstCommitTime.Format(time.DateOnly)
	c.Data["RangeEndStr"] = latestCommitTime.Format(time.DateOnly)
	c.Data["DefaultBranch"] = defaultBranch
	switch ctxQueryType {
	case contributionTypeAddition:
		c.Data["ContributorsMainChartData"] = contributorsMainChartData.Additions
	case contributionTypeDeletion:
		c.Data["ContributorsMainChartData"] = contributorsMainChartData.Deletions
	default:
		c.Data["ContributorsMainChartData"] = contributorsMainChartData.Commits
	}
	c.Data["RequireChartJS"] = true

	c.RequireAutosize()
	c.Success(INSIGHT_CONTRIBUTORS)
}

// // InsightCommitsPage represents the GET method for the commits insight page.
// func InsightCommitsPage(c *context.Context) {
// 	c.Title("repo.insights.commits")
// 	c.PageIs("InsightsCommits")
// 	c.RequireAutosize()
// 	c.Success(INSIGHT_COMMITS)
// }

// // InsightCodeFrequencyPage represents the GET method for the code frequency insight page.
// func InsightCodeFrequencyPage(c *context.Context) {
// 	c.Title("repo.insights.code_frequency")
// 	c.PageIs("InsightsCodeFrequency")
// 	c.RequireAutosize()
// 	c.Success(INSIGHT_CODE_FREQUENCY)
// }

// InsightsGroup represents the handler for the insights group.
func InsightsGroup(c *context.Context) {
	c.PageIs("Insights")
}

// getContributorsMainChartData returns the ContributorsMainChartData struct that
// will be used in page's template. It takes a slice of *commitData and returns a
// ContributorsMainChartData struct. The ContributorsMainChartData struct contains
// three ChartData structs: Additions, Deletions, and Commits. Each ChartData struct
// represents a dataset for the chart, with labels and data.
//
// NOTE: The input commits slice is expected to be already sorted by commit time.
func getContributorsMainChartData(commits []*commitData) ContributorsMainChartData {
	commitsByDay := groupCommitsByDay(commits)
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

	return ContributorsMainChartData{
		Additions: additionChartData,
		Deletions: deletionChartData,
		Commits:   commitChartData,
	}
}

// getCommitData returns a slice of commitData structs. Each commitData struct
// contains a *git.Commit and the number of additions and deletions made in that
// commit.
func getCommitData(c *context.Context, branch string) ([]*commitData, error) {
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

		diff, _ := gitutil.RepoDiff(c.Repo.GitRepo,
			endCommitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
			git.DiffOptions{Base: startCommitID, Timeout: time.Duration(conf.Git.Timeout.Diff) * time.Second},
		)
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
func groupCommitsByDay(commits []*commitData) [][]*commitData {
	res := make([][]*commitData, 0)

	firstCommitTime := commits[len(commits)-1].Commit.Author.When.Truncate(24 * time.Hour)
	latestCommitTime := commits[0].Commit.Author.When.Truncate(24 * time.Hour)
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

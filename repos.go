package main

type Repos interface {
	Url(pkgName string, ver string) string
}

type GithubRepos interface {
}

type GitLabRepos interface {
}

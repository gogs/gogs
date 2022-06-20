# Welcome to Gogs contributing guide

Thank you for investing your time in contributing to our projects!

Read our [Code of Conduct](https://go.dev/conduct) to keep our community approachable and respectable.

In this guide you will get an overview of the contribution workflow from opening an issue, creating a PR, reviewing, and merging the PR.

Use the table of contents icon <img src="https://github.com/github/docs/raw/50561895328b8f369694973252127b7d93899d83/assets/images/table-of-contents.png" width="25" height="25" /> on the top left corner of this document to get to a specific section of this guide quickly.

## New contributor guide

To get an overview of the project, read the [README](/README.md). Here are some resources to help you get started with open source contributions:

- [Finding ways to contribute to open source on GitHub](https://docs.github.com/en/get-started/exploring-projects-on-github/finding-ways-to-contribute-to-open-source-on-github)
- [Set up Git](https://docs.github.com/en/get-started/quickstart/set-up-git)
- [GitHub flow](https://docs.github.com/en/get-started/quickstart/github-flow)
- [Collaborating with pull requests](https://docs.github.com/en/github/collaborating-with-pull-requests)
- [How to Contribute to Open Source](https://opensource.guide/how-to-contribute/)
- [Talk, then code](https://www.craft.do/s/kyHVs6OoE4Dj5V)

In addition to the general guides with open source contributions, you would also need to:

- Have basic knowledge about web applications development, database management systems and programming in [Go](https://go.dev/).
- Have a working local development setup with a reasonable good IDE or editor like [Visual Studio Code](https://code.visualstudio.com/docs/languages/go), [GoLand](https://www.jetbrains.com/go/) or [Vim](https://github.com/fatih/vim-go).
- [Set up your development environment](/docs/dev/local_development.md).

## Issues

### Ask for help

Before opening an issue, please make sure the problem you're encountering isn't already addressed on the [Troubleshooting](https://gogs.io/docs/intro/troubleshooting.html) and [FAQs](https://gogs.io/docs/intro/faqs.html) pages.

### Create a new issue

- For questions, ask in [Discussions](https://github.com/gogs/gogs/discussions).
- [Check to make sure](https://docs.github.com/en/github/searching-for-information-on-github/searching-on-github/searching-issues-and-pull-requests#search-by-the-title-body-or-comments) someone hasn't already opened a similar [issue](https://github.com/gogs/gogs/issues).
- If a similar issue doesn't exist, open a new issue using a relevant [issue form](https://github.com/gogs/gogs/issues/new/choose).
- Blank issues that are not coming from maintainers will be closed without a response.

### Pick up an issue to solve

- Scan through our [existing issues](https://github.com/gogs/gogs/issues) to find one that interests you.
- The [👋 good first issue](https://github.com/gogs/gogs/issues?q=is%3Aissue+is%3Aopen+label%3A%22%F0%9F%91%8B+good+first+issue%22) is a good place to start exploring issues that are well-groomed for newcomers.
- Do not hesitate to ask for more details or clarifying questions on the issue!
- Communicate on the issue you are intended to pick up _before_ starting working on it.
- Every issue that gets picked up will have an expected timeline for the implementation, the issue may be reassigned after the expected timeline. Please be responsible and proactive on the communication 🙇‍♂️

## Add new features or make big changes

New features or big changes require proposals before we may be able to accept any contribution. Proposals should be posted to the [Discussions - Proposal](https://github.com/gogs/gogs/discussions/categories/proposal) category for review and discussions. GitHub Discussions provides sub-threading which is much more suitable than GitHub Issues for discussions to happen. Please read [Write a proposal for open source contributions](https://unknwon.io/posts/220210-write-a-proposal-for-open-source-contributions/) to begin with.

## Pull requests

When you're finished with the changes, create a pull request, or a series of pull requests if necessary.

Contributing to another codebase is not as simple as code changes, it is also about contributing influence to the design. Therefore, we kindly ask you that:

- Please acknowledge that no pull request is guaranteed to be merged.
- Please always do a self-review before requesting reviews from others.
- Please expect code review to be strict and may have multiple rounds.
- Please make self-contained incremental changes, pull requests with huge diff may be rejected for review.
- Please use English in code comments and docstring.
- Please do not force push unless absolutely necessary. Force pushes make review much harder in multiple rounds, and we use [Squash and merge](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/about-pull-request-merges#squash-and-merge-your-pull-request-commits) so you don't need to worry about messy commits and just focus on the changes.

### Things we do not accept

1. Updates to locale files (`conf/locale_xx-XX.ini`) other than the `conf/locale_en-US.ini`. Please read the [guide for localizing Gogs](https://gogs.io/docs/features/i18n).
1. Docker compose files.

### Coding guidelines

1. Please read the Sourcegraph's [Go style guide](https://docs.sourcegraph.com/dev/background-information/languages/go).
1. **NO** direct modifications to `.css` files, `.css` files are all generated by `.less` files. You can regenerate `.css` files by executing `task less`.

## Your PR is merged!

Congratulations 🎉🎉 Thanks again for taking the effort to have this journey with us 🌟

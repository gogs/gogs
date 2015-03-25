# Contributing to Gogs

> This guidelines sheet is forked from [CONTRIBUTING.md](https://github.com/drone/drone/blob/8d9c7cee56d6c2eac81dc156ce27be6716d97e68/CONTRIBUTING.md).

Gogs is not perfect, and it has bugs or incomplete features in rare cases. You're welcome to tell us, or to contribute some code. This document describes details about how can you contribute to Gogs project.

## Contribution guidelines

Depends on the situation, you will:

- Find a bug and create an issue
- Need more functionality and make a feature request
- Want to contribute code and open a pull request
- Run into issue and need help

### Bug Report

If you find something you consider a bug, please create a issue on [GitHub](https://github.com/gogits/gogs/issues). To avoid wasting time and reduce back-and-forth communication with team members, please include at least the following information in a form comfortable for you:

- Bug Description
- Gogs Version
- Git Version
- System Type
- Error Log
- Other information

Please take a moment to check that an issue on [GitHub](https://github.com/gogits/gogs/issues) doesn't already exist documenting your bug report or improvement proposal. If it does, it never hurts to add a quick "+1" or "I have this problem too". This will help prioritize the most common problems and requests.

#### Bug Report Example

Gogs crashed when creating a repository with a license, using v0.5.13.0207, SQLite3, Git 1.9.0, Ubuntu 12.04.

Error log:

```
2014/09/01 07:21:49 [E] nil pointer
```

### Feature Request

There is no standard form of making a feature request. Just try to describe the feature as clearly as possible, because team members may not have experience with the functionality you're talking about.

### Pull Request

Pull requests are always welcome, but note that **ALL PULL REQUESTS MUST APPLY TO THE `DEV` BRANCH**.

We are always thrilled to receive pull requests, and do our best to process them as fast as possible. Not sure if that typo is worth a pull request? Do it! We will appreciate it.

If your pull request is not accepted on the first try, don't be discouraged! If there's a problem with the implementation, hopefully you received feedback on what to improve.

We're trying very hard to keep Gogs lean and focused. We don't want it to do everything for everybody. This means that we might decide against incorporating a new feature. We believe you do like to discuss with us first in [Gitter](https://gitter.im/gogits/gogs).

### Ask For Help

Before opening an issue, please make sure your problem isn't already addressed on the [Troubleshooting](http://gogs.io/docs/intro/troubleshooting.md) and [FAQs](http://gogs.io/docs/intro/faqs.html) pages.

## Things To Notice

Please take a moment to check that an issue on [GitHub](https://github.com/gogits/gogs/issues) or card on [Trello](https://trello.com/b/uxAoeLUl/gogs-go-git-service) doesn't already exist documenting your bug report or improvement proposal. If it does, it never hurts to add a quick "+1" or "I have this problem too". This will help prioritize the most common problems and requests.

## Code of conduct

As contributors and maintainers of this project, we pledge to respect all people who contribute through reporting issues, posting feature requests, updating documentation, submitting pull requests or patches, and other activities.

We are committed to making participation in this project a harassment-free experience for everyone, regardless of level of experience, gender, gender identity and expression, sexual orientation, disability, personal appearance, body size, race, age, or religion.

Examples of unacceptable behavior by participants include the use of sexual language or imagery, derogatory comments or personal attacks, trolling, public or private harassment, insults, or other unprofessional conduct.

Project maintainers have the right and responsibility to remove, edit, or reject comments, commits, code, wiki edits, issues, and other contributions that are not aligned to this Code of Conduct. Project maintainers who do not follow the Code of Conduct may be removed from the project team.

Instances of abusive, harassing, or otherwise unacceptable behavior can be reported by emailing u@gogs.io

This Code of Conduct is adapted from the [Contributor Covenant](http:contributor-covenant.org), version 1.0.0, available at [http://contributor-covenant.org/version/1/0/0/](http://contributor-covenant.org/version/1/0/0/)

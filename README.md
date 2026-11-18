
 <p>
  <div align="center">
    <img src="docs/images/logo-light.svg#gh-light-mode-only" alt="Gogs">
    <img src="docs/images/logo-dark.svg#gh-dark-mode-only" alt="Gogs">
  </div>
  <div align="center">
    <a href="https://github.com/gogs/gogs/actions?query=branch%3Amain"><img
  src="https://img.shields.io/github/checks-status/gogs/gogs/main?logo=github&style=for-the-badge" alt="GitHub Workflow Status"></a>
    <a href="https://sourcegraph.com/github.com/gogs/gogs"><img
src="https://img.shields.io/badge/view%20on-Sourcegraph-brightgreen.svg?style=for-the-badge&logo=sourcegraph" alt="Sourcegraph"></a>
  </div>
  <div align="center">
    👉 Deploy on DigitalOcean and <a href="https://m.do.co/c/5aeb02268b55">get $200 in free credits</a>!
  </div>
</p>

## 🔮 Vision

The Gogs (`/gɑgz/`) project aims to build a simple, stable and extensible self-hosted Git service that can be set up in the most painless way. With Go, this can be done with an independent binary distribution across all platforms that Go supports, including Linux, macOS, Windows and ARM-based systems.

## 📡 Overview

- Please visit [our home page](https://gogs.io) for user documentation.
- Please refer to [CHANGELOG.md](CHANGELOG.md) for list of changes in each releases.
- Want to try it before doing anything else? Do it [online](https://try.gogs.io/gogs/gogs)!
- Having trouble? Help yourself with [troubleshooting](https://gogs.io/asking/troubleshooting) or ask questions in [Discussions](https://github.com/gogs/gogs/discussions).
- Want to help with localization? Check out the [localization documentation](https://gogs.io/advancing/localization).
- Ready to get hands dirty? Read our [contributing guide](.github/CONTRIBUTING.md).
- Hmm... What about APIs? We have experimental support with [documentation](https://gogs.io/api-reference).

## 💌 Highlights

- Code hosting
  - Git over SSH and HTTP(S) with protocol v2, including a built-in SSH server.
  - SHA-256 repositories, deploy keys and LFS (local or S3\*).
  - Forks, mirrors and migration from other code hosts, with GitHub two-way sync\*.
- Code browsing
  - Browse and edit code in the browser, with blame\*.
  - Code search within a repository or across all of them.
  - Rendered Markdown, Jupyter Notebooks, PDFs and Mermaid diagrams.
  - Rich diffs for Markdown and images.
- Collaboration
  - Issues with templates and forms.
  - Pull requests with merge commit, rebase and squash merges, inline reviews\* and stacks\*.
  - Protected branches, with code owners\* and signed commits\*.
  - Wikis, releases and projects\*.
  - Organizations, teams, collaborators and service accounts\*.
- Automation
  - Actions\* and compute\*.
  - Packages\* and hosting\*.
- Interface
  - Dashboards, profiles and activity timelines.
  - A command palette for navigation, search and actions.
  - Light and dark themes, and over [31 languages](https://crowdin.com/project/gogs).
- Integrations
  - Webhooks for Slack, Discord, DingTalk and more, plus server-side Git hooks.
  - A REST [API](https://gogs.io/api-reference), GitHub-compatible\*.
- Security
  - Passkeys and multi-factor authentication.
  - Sign in with LDAP, SMTP, PAM, reverse proxy, OAuth 2.0\*, OpenID Connect\* or SAML 2.0\*.
  - Session management\*, security alerts\* and breached password detection\*.
- Operations
  - Single binary, Docker or Kubernetes\*, on Linux, macOS, Windows and ARM.
  - PostgreSQL, MySQL or SQLite3.
  - OpenTelemetry tracing with a built-in trace viewer, and Prometheus metrics.
  - Backup and restore from the command line.

<sub>* In development.</sub>

## 💾 Hardware requirements

- A Raspberry Pi or $5 Digital Ocean Droplet is more than enough to get you started. Some even use 64MB RAM Docker [CaaS](https://www.docker.com/blog/containers-as-a-service-caas/).
- 2 CPU cores and 512MB RAM would be the baseline for teamwork.
- Increase CPU cores when your team size gets significantly larger, memory footprint remains low.

## 💻 Browser support

- Any modern browser works, with Chrome 111, Edge 111, Firefox 128 and Safari 16.4 (including iOS) as the minimum versions.
- The UI is mobile-friendly and works on screens as narrow as 375px.
- WebAssembly SIMD makes syntax highlighting in diffs faster, but is not required.

## 🙇‍♂️ Acknowledgments

<p>This project is proudly supported by:</p>
<p>
  <a href="https://m.do.co/c/5aeb02268b55">
    <img src="https://opensource.nyc3.cdn.digitaloceanspaces.com/attribution/assets/SVG/DO_Logo_horizontal_blue.svg" width="201px">
  </a>
</p>

Other acknowledgments:

- Thanks [Egon Elbre](https://twitter.com/egonelbre) for designing the original version of the logo.
- Thanks [Mintlify](https://mintlify.com) for sponsoring open source documentation plan.
- Thanks [Crowdin](https://crowdin.com) for sponsoring open source translation plan.
- Thanks [Buildkite](https://buildkite.com) for sponsoring open source CI/CD plan.

## ⚖️ License

This project is under the MIT License. See the [LICENSE](https://github.com/gogs/gogs/blob/main/LICENSE) file for the full license text.

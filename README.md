
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

## 💌 Features

- Code hosting
  - Clone and push over SSH, HTTP and HTTPS, with an optional built-in SSH server.
  - Git LFS with local or S3-compatible\* storage, deploy keys and server-side Git hooks.
  - Migrate and mirror repositories from other code hosts.
  - Edit files right in the browser.
  - Render Markdown, Jupyter Notebooks and PDFs.
  - Search code in one repository or across all of them.\*
  - See who last changed each line with blame.\*
  - Compare images and rendered Markdown in diffs.\*
- Collaboration
  - Issues, wikis and pull requests, with inline reviews\* and stacks\*.
  - Organizations, teams and per-repository collaborators.
  - Dashboards, user profiles and activity timelines.
  - Protected branches, with required code owner reviews\* and signed commits\*.
  - Work with repositories across federated Gogs instances.\*
- Interface
  - Light and dark themes.
  - Jump to repositories, files, issues and code from a command palette.\*
- Integrations
  - Repository and organization webhooks, with native Slack, Discord and DingTalk support.
  - A REST [API](https://gogs.io/api-reference) for automation, with GitHub compatibility\*.
- Security
  - Sign in with LDAP, SMTP, PAM, reverse proxy, OAuth 2.0\*, OpenID Connect\* or SAML 2.0\*.
  - Multi-factor authentication with TOTP, recovery codes and passkeys\*.
  - Confirm who you are before sensitive actions.\*
  - See and sign out the browsers signed in to your account.\*
  - Get emails about changes to the security of your account.\*
  - Require a minimum password length and optionally reject leaked passwords.\*
  - Block pushes with secrets, get alerts for vulnerable dependencies, and see static analysis results.\*
- Operations
  - Run a single binary on Linux, macOS, Windows and ARM, the official Docker image, or scale out on Kubernetes\*.
  - PostgreSQL, MySQL, MariaDB or SQLite3, plus anything that speaks one of their protocols.
  - Customize templates, static files and much more, and preview emails in the admin panel\*.
  - Monitor with OpenTelemetry and Prometheus, plus a built-in trace view in the admin panel\*.
  - Available in over [31 languages](https://crowdin.com/project/gogs), including emails\*.

<sub>* In development.</sub>

## 💾 Hardware requirements

- A Raspberry Pi or $5 Digital Ocean Droplet is more than enough to get you started. Some even use 64MB RAM Docker [CaaS](https://www.docker.com/blog/containers-as-a-service-caas/).
- 2 CPU cores and 512MB RAM would be the baseline for teamwork.
- Increase CPU cores when your team size gets significantly larger, memory footprint remains low.

## 💻 Browser support

- Any modern browser works, with Chrome 111, Edge 111, Firefox 128 and Safari 16.4 (including iOS) as the minimum versions.
- The UI is mobile-friendly and works on screens as narrow as 375px.

## 📜 Installation

Please follow [the guide in our documentation](https://gogs.io/getting-started/installation).

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

## 👋 Contributors

- See [contributors page](https://github.com/gogs/gogs/graphs/contributors) for top 100 contributors.
- See [TRANSLATORS](conf/locale/TRANSLATORS) for public list of translators.

## ⚖️ License

This project is under the MIT License. See the [LICENSE](https://github.com/gogs/gogs/blob/main/LICENSE) file for the full license text.

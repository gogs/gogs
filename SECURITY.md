# Security policy

## Supported versions

Only lastest two minor version releases are supported (>= 0.12) for accepting vulnerability reports and patching fixes.

Existing vulnerability reports are being tracked in [Gogs Vulnerability Reports](https://jcunknwon.notion.site/Gogs-Vulnerability-Reports-81d7df52e45c4f159274e46ba48ed1b9).

## Vulnerability lifecycle

1. Report a vulnerability:
    - We strongly enourage to use https://huntr.dev/ for submitting and managing status of vulnerability reports.
    - Alternatively, you may send vulnerability reports through emails to [security@gogs.io](mailto:security@gogs.io).
1. Create a [dummy issue](https://github.com/gogs/gogs/issues/6810) with high-level description of the security vulnerability for credibility and tracking purposes.
1. Project maintainers review the report and either:
    - Ask clarifying questions
    - Confirm or deny the vulnerability
1. Once the vulnerability is confirmed, the reporter may submit a patch or wait for project maintainers to patch.
    - The latter is usually significantly slower.
1. Patch releases will be made for the supported versions.
1. Publish the original vulnerability report and a new [GitHub security advisory](https://github.com/gogs/gogs/security/advisories).

Thank you!

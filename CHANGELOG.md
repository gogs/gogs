# Changelog

All notable changes to Gogs are documented in this file.

## 0.12.0+dev (`master`)

### Added

- Allow admin to remove observers from the repository. [#5803](https://github.com/gogs/gogs/pull/5803)
- Use `Last-Modified` HTTP header for raw files. [#5811](https://github.com/gogs/gogs/issues/5811)
- Support syntax highlighting for SAS code files (i.e. `.r`, `.sas`, `.tex`, `.yaml`). [#5856](https://github.com/gogs/gogs/pull/5856)
- Able to fill in pull request title with a template. [#5901](https://github.com/gogs/gogs/pull/5901)
- Able to override static files under `public/` directory, please refer to [documentation](https://gogs.io/docs/features/custom_template) for usage. [#5920](https://github.com/gogs/gogs/pull/5920)

### Changed

- All assets are now embedded into binary and served from memory by default. Set `[server] LOAD_ASSETS_FROM_DISK = true` to load them from disk. [#5920](https://github.com/gogs/gogs/pull/5920)

### Fixed

- [Security] Potential open redirection with i18n.
- [Security] Potential RCE on mirror repositories. [#5767](https://github.com/gogs/gogs/issues/5767)
- [Security] Potential XSS attack with raw markdown API. [#5907](https://github.com/gogs/gogs/pull/5907)
- Open/close milestone redirects to a 404 page. [#5677](https://github.com/gogs/gogs/issues/5677)
- Disallow multiple tokens with same name. [#5587](https://github.com/gogs/gogs/issues/5587) [#5820](https://github.com/gogs/gogs/pull/5820)
- Enable Federated Avatar Lookup could cause server to crash. [#5848](https://github.com/gogs/gogs/issues/5848)
- Private repositories are hidden in the organization's view. [#5869](https://github.com/gogs/gogs/issues/5869)
- Server error when changing email address in user settings page. [#5899](https://github.com/gogs/gogs/issues/5899)

### Removed

---

**Older change logs can be found on [GitHub](https://github.com/gogs/gogs/releases?after=v0.12.0).**

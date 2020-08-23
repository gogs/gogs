# Release strategy

## Semantic versioning

Starting 0.12.0, Gogs uses [semantic versioning](https://semver.org/) for publishing releases. For example:

- `0.12.0` is a minor version release.
- `0.12.1` is the first patch release of `0.12`.
- `0.12` indicates a series of releases for a minor version and its patch releases.

Each minor release has its own release branch with prefix `release/`, e.g. `release/0.12` is the release branch for minor version 0.12.0 and all its patch releases (`0.12.1`, `0.12.2`, etc.).

## Backwards compatibility

### Before 0.12

If you're running Gogs with any version below 0.12, please upgrade to 0.12 to run necessary migrations.

### After 0.12

We maintain one minor version backwards compatibility, patch releases are disregarded.

For example, you should:

- Upgrade from `0.12.0` to `0.13.0`.
- Upgrade from `0.12.1` to `0.13.4`.
- NOT upgrade from `0.12.4` to `0.14.0`.

Therefore, we recommend upgrade one minor version at a time.

### Running source builds

If you're running Gogs with building from source code, we recommend you update at least weekly to be not fall behind and potentially miss migrations.

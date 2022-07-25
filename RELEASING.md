# Creating a new release

1. Update `version.go` with new layer version.
1. Update `README.md` with new layer version.
1. Add new entry in `CHANGELOG.md`, include both release version and layer version.
1. Open a PR for release prep.
1. Once the above changes are merged into `main`, tag `main` with the new release version, e.g. `v0.1.1`. Push the tag. This will kick off CI, which will create a draft GitHub release and publish the new layer version in AWS.
1. Update release notes on the new draft GitHub release with notes from changelog and Layer Version ARN, and publish that.
1. Prep docs PR for updated layer version and release version. **NOTE**: layer version and release version are totally different :(
1. Merge PR in public docs with the new version.

Voila!

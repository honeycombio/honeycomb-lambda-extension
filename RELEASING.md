# Creating a new release

1. Update `version.go`, add new entry in the changelog. Update readme instructions with the new version. NOTE: layer version and release version are totally different :(

2. Once the above changes are merged into `main`, tag `main` with the new version, e.g. `v0.1.1`. Push the tags. This will kick off CI, which will create a draft GitHub release, and publish the new layer version in AWS.

3. Update release notes on the new draft GitHub release, and publish that.

4. Update public docs and repo README with the new layer version.

Voila!

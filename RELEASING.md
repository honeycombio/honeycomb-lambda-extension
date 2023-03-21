# Creating a new release

1. Use [go-licenses](https://github.com/google/go-licenses) to ensure all project dependency licenses are correclty represented in this repository:
  1. Install go-licenses (if not already installed) `go install github.com/google/go-licenses@latest`
  1. Run and save licenses `go-licenses save github.com/honeycombio/buildevents --save_path="./LICENSES"`
  1. If there are any changes, submit PR to update licenses.
1. Draft a docs PR for this release.
1. Update `CHANGELOG.md` with the changes since the last release.
1. Commit changes, push, and open a release preparation PR for review.
1. Once the pull request is merged, fetch the updated main branch.
1. Apply a tag for the new version on the merged commit (e.g. `git tag -a v2.3.1 -m "v2.3.1"`)
1. Push the tag upstream to kick off the release pipeline in CI (e.g. `git push origin v2.3.1`). This will create a draft GitHub release with build artifacts and will publish the new layer version in AWS.
1. Craft a release.json for this release. Most the content for a `release.json` appears in the output of the publish_aws CI job.
1. Edit the draft GitHub release:
    - Click the Generate Release Notes button and double-check the content against the CHANGELOG.
    - Attach the updated `release.json` to the release as a "binary".
1. Return to the docs PR and update `data/projects/honeycomb-lambda-extension/release.json` and get a review!

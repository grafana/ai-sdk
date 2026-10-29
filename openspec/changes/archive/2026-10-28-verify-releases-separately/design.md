# Design

## Decisions

- **Separate workflow on `release: published`.** GitHub Releases are created with
  the release App token, so they trigger workflows. One run per tag keeps waits
  independent and out of the release workflow's non-cancelling concurrency group,
  which would otherwise delay the next release run. Manual dispatch reruns a tag.
  The job is read-only (`contents: read`) and passes the tag through an environment
  variable that `release_consumer.py` validates.
- **Wait for availability, then verify once.** Resolving the version and its
  first-party requirements with `go mod download -json` distinguishes "not
  available yet" from a broken release regardless of the error wording. The
  consumer build and scenarios run once after everything resolves.
- **Skip preparation when `main` advanced.** The push that advanced `main` has its
  own queued release run, which prepares from the newer base. A base change during
  preparation writes still fails the run.

## Risks / Trade-offs

- A release that never becomes resolvable fails after 40 minutes; the run marks it
  for a fix-forward release as before.

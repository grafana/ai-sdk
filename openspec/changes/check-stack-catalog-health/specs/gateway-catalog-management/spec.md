# Gateway Catalog Management Delta

## ADDED Requirements

### Requirement: Check the saved catalog from its page
The saved catalog page SHALL offer an explicit **Check health** action only to a caller who may read the catalog and has the `modelcatalogs/health` `get` permission. The page SHALL NOT check as part of loading, saving or creation, or from the list or overview. It SHALL prevent duplicate submissions while checking and report route failures separately from probe results.

#### Scenario: Writer checks a catalog
- **WHEN** a caller with both permissions clicks **Check health** on a saved catalog
- **THEN** the page SHALL request one health check for that catalog and display its progress without modifying the catalog

#### Scenario: Caller may read or invoke but may not run health
- **WHEN** a caller without health permission opens a stored catalog
- **THEN** the page SHALL neither show the control nor request health results

### Requirement: Show results only for the saved revision
The saved catalog page SHALL show provider status next to, but distinct from, key storage state, primary target status in the model row, and fallback statuses in model details. It SHALL explain that results reflect model listings, not an inference test, and SHALL use fixed UI labels. It SHALL discard results that do not match the displayed catalog revision and clear the result when that revision changes.

#### Scenario: Catalog revision changes while the check is running
- **WHEN** a check returns for an older revision than the page is displaying
- **THEN** the page SHALL NOT put those statuses on the edited provider or model rows

#### Scenario: Listing accepts one key but another fails
- **WHEN** a response lists the primary target but rejects a saved fallback provider's key
- **THEN** the page SHALL show the primary as listed and the fallback as not checked, while the fallback provider shows both its saved key state and the separate key rejection

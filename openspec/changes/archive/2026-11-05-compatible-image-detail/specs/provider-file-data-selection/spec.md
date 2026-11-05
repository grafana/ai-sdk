## ADDED Requirements

### Requirement: Compatible typed image detail

OpenAI-compatible SHALL accept a typed per-image detail option and map it to
image_url.detail while deriving the URL from the common file data. The semantic
imageDetail field SHALL NOT leak into native content-part extensions. Unknown
detail values SHALL fail before HTTP; other metadata and source options SHALL
remain unchanged. This is a documented Go extension to the pinned converter.

#### Scenario: Typed detail preserves the image address
- **WHEN** an image file supplies auto, low, high or original detail
- **THEN** unary and streaming native requests contain the selected nested detail
  with the original URL, and the caller's options remain unchanged

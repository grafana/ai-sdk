## ADDED Requirements

### Requirement: User multimodal inputs

The adapter SHALL map user image URLs, inline audio and file data or IDs into
common file parts, retaining order and inline filenames. Per-attempt translation
SHALL select the native reference namespace without mutating the source prompt.
Image detail SHALL use a typed compatible option, never a raw URL override.
A native transport unable to represent the input SHALL fail before native I/O.

#### Scenario: Images and PDF across backends
- **WHEN** a user sends images, PDF data and an uploaded file ID
- **THEN** Responses, Anthropic and compatible requests retain their native media
  representations; Responses and compatible requests retain image detail.

#### Scenario: Audio capability
- **WHEN** a user sends WAV or MP3 audio
- **THEN** a compatible Chat attempt forwards input_audio and a Responses or
  Anthropic attempt fails without sending or discarding the audio.

#### Scenario: Protected image URL
- **WHEN** a compatible input supplies typed image detail
- **THEN** the native image URL still comes from the common file part and raw
  image_url, type, file and input_audio overrides remain refused by Gateway policy.

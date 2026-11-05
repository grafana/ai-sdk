# Compatible image detail without native URL replacement

## Why
Chat clients specify image resolution inside image_url. Gateway policy correctly refuses replacing builder-owned image URLs via opaque metadata, so a typed option is needed to retain detail safely.

## What Changes
Add ImageOptions and ImageDetail to the compatible provider. Consume imageDetail locally and write the nested detail field without changing the file URL. Preserve unrelated metadata and existing direct-provider raw overlay semantics.

## Impact
Provider input projection and documentation. The pinned compatible 3.0.82 converter exposes only raw metadata spread; this typed mapping is an intentional Go extension. Official openai-go3.76 supports auto, low, high and original modes. No recorded input or baseline is changed.

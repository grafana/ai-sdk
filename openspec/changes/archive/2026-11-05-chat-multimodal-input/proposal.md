# Chat multimodal user input

## Why
Official Chat clients send image_url, input_audio and file parts. The adapter rejected those requests even when the selected native provider could represent them.

## What Changes
Decode user media into common file parts. Preserve mixed order, inline filenames, image resolution and uploaded references. Translate reference namespaces and compatible image detail per native attempt. Reject malformed data and native transport capability gaps before native I/O.

## Impact
Chat input mapping, shared compatible image options, guard regression tests and user documentation. Selection, model construction, native conversion and output lifecycle remain in their existing owners. Audio uses compatible Chat because the current Anthropic and Responses transports cannot represent Chat input_audio. No downloads, uploads or generated provider recordings are introduced.

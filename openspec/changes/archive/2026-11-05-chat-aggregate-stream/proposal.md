# Validate the public aggregate
Independent review found that validating native segments separately permits invalid
concatenated JSON and too many tool calls. Apply public constraints after the runtime
finishes all segments, preserve tool-call terminal semantics and global tool indices.

package kinds

storedChatCompletion: {
 kind: "StoredChatCompletion"
 plural: "storedchatcompletions"
 folderScoped: false
 current: "v1alpha1"
 versions: v1alpha1: {
  schema: spec: {
   owner: string
   id: string
   created: int64
   expiresAt: int64
   model: string
   // JSON contains the sanitized public record, never provider credentials.
   record: string
  }
 }
}

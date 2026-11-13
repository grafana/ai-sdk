# Middleware manifest fixture

`generated-middleware-schemas.json` is the `Middleware` kind schema that
grafana-app-sdk v0.60.16 generates for a hand-written `modelAccess` member.
`TestMemberProblems` uses it to prove the member consistency check against real
generator output, independently of the shipped tenant types.

To regenerate it, for example after an app-sdk upgrade, copy `kinds/`,
`.config/app-sdk/`, `go.mod` and `go.sum` to a scratch directory, replace
`kinds/middleware_members.cue` there with the member below, run
`node .config/app-sdk/generate-kinds.mjs`, and copy the `Middleware` kind's
`schemas` object from the generated `src/app-sdk-manifest.json`.

```cue
package kinds

middlewareMembers: {
	// Controls which models teams may use.
	modelAccess?: {
		// Whether rules allow or deny.
		// Applies to every rule.
		mode?: string
		rules: [...{
			limits?: {
				rpm?: int
			}
			models: [...string]
			teams: [...string]
		}]
		strict?:      bool
		temperature?: number
	}
}
```

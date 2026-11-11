// Command members writes the Middleware kind's members from the public tenant
// types, for the app-sdk generator.
package main

import (
	"fmt"
	"os"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/tenantschema"
)

func main() {
	file, err := tenantschema.Render(tenanttype.All())
	if err == nil {
		err = os.WriteFile(tenantschema.MembersFile, file, 0o644)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "members:", err)
		os.Exit(1)
	}
}

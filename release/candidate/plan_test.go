package candidate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

var testModules = []Module{
	{Dir: ".", Path: CorePath},
	{Dir: "providers/openai", Path: CorePath + "/providers/openai"},
	{Dir: "providers/bedrock", Path: CorePath + "/providers/bedrock"},
	{Dir: "middleware/logger", Path: CorePath + "/middleware/logger"},
	{Dir: GatewayDir, Path: CorePath + "/" + GatewayDir},
}

func targetDirs(plan Plan) []string {
	dirs := make([]string, 0, len(plan.Targets))
	for _, m := range plan.Targets {
		dirs = append(dirs, m.Dir)
	}
	return dirs
}

func TestNewPlan(t *testing.T) {
	tests := []struct {
		name    string
		before  map[string]string
		after   map[string]string
		check   func(t *testing.T, plan Plan)
		wantErr string
	}{
		{
			name:   "root and bedrock without openai",
			before: map[string]string{".": "0.1.0-alpha.3", "providers/openai": "0.1.0-alpha.1", GatewayDir: "0.1.0-alpha.1"},
			after:  map[string]string{".": "0.1.0-alpha.4", "providers/openai": "0.1.0-alpha.1", "providers/bedrock": "0.1.0-alpha.1", GatewayDir: "0.1.0-alpha.1"},
			check: func(t *testing.T, plan Plan) {
				assert.Equal(t, map[string]string{CorePath: "v0.1.0-alpha.4", CorePath + "/providers/bedrock": "v0.1.0-alpha.1"}, plan.Released)
				assert.Equal(t, []string{".", "providers/bedrock", GatewayDir}, targetDirs(plan))
				version, err := plan.Version(CorePath + "/providers/openai")
				require.NoError(t, err)
				assert.Equal(t, "v0.1.0-alpha.1", version)
				_, err = plan.Version(CorePath + "/" + GatewayDir)
				assert.ErrorIs(t, err, ErrUntaggedPrerequisite)
			},
		},
		{
			name:   "nothing released still targets gateway",
			before: map[string]string{".": "0.1.0-alpha.3"},
			after:  map[string]string{".": "0.1.0-alpha.3"},
			check: func(t *testing.T, plan Plan) {
				assert.Empty(t, plan.Released)
				assert.Equal(t, []string{GatewayDir}, targetDirs(plan))
			},
		},
		{
			name:    "gateway release",
			before:  map[string]string{GatewayDir: "0.1.0-alpha.1"},
			after:   map[string]string{GatewayDir: "0.1.0-alpha.2"},
			wantErr: "Gateway is released by its own pull request",
		},
		{
			name:    "unregistered component",
			before:  map[string]string{},
			after:   map[string]string{"providers/unknown": "0.1.0-alpha.1"},
			wantErr: `manifest component "providers/unknown" is not a registered module`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := NewPlan(testModules, tc.before, tc.after)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, plan)
		})
	}
}

func TestPlanRewriteRequirements(t *testing.T) {
	plan, err := NewPlan(testModules,
		map[string]string{".": "0.1.0-alpha.3", "providers/openai": "0.1.0-alpha.1", "middleware/logger": "0.1.0-alpha.1", GatewayDir: "0.1.0-alpha.1"},
		map[string]string{".": "0.1.0-alpha.4", "providers/openai": "0.1.0-alpha.1", "providers/bedrock": "0.1.0-alpha.1", "middleware/logger": "0.1.0-alpha.1", GatewayDir: "0.1.0-alpha.1"})
	require.NoError(t, err)

	tests := []struct {
		name        string
		goMod       string
		want        string
		wantChanged bool
		wantErr     error
	}{
		{
			name: "bedrock requires co-released root and latest openai",
			goMod: `module github.com/grafana/ai-sdk/providers/bedrock

go 1.27.3

require (
	github.com/aws/aws-sdk-go-v2 v1.41.0
	github.com/grafana/ai-sdk v0.1.0-alpha.2
	github.com/grafana/ai-sdk/providers/openai v0.0.0-20260928202004-dd43c6dff058
)
`,
			want: `module github.com/grafana/ai-sdk/providers/bedrock

go 1.27.3

require (
	github.com/aws/aws-sdk-go-v2 v1.41.0
	github.com/grafana/ai-sdk v0.1.0-alpha.4
	github.com/grafana/ai-sdk/providers/openai v0.1.0-alpha.1
)
`,
			wantChanged: true,
		},
		{
			name: "gateway pins follow co-released libraries, including indirect requirements",
			goMod: "module " + CorePath + "/" + GatewayDir + `

go 1.27.3

require (
	// keep this comment
	github.com/grafana/ai-sdk v0.1.0-alpha.3
	github.com/grafana/ai-sdk/middleware/logger v0.0.0-20260928202004-dd43c6dff058
)

require github.com/grafana/ai-sdk/providers/bedrock v0.0.0-20260928202004-dd43c6dff058 // indirect
`,
			want: "module " + CorePath + "/" + GatewayDir + `

go 1.27.3

require (
	// keep this comment
	github.com/grafana/ai-sdk v0.1.0-alpha.4
	github.com/grafana/ai-sdk/middleware/logger v0.1.0-alpha.1
)

require github.com/grafana/ai-sdk/providers/bedrock v0.1.0-alpha.1 // indirect
`,
			wantChanged: true,
		},
		{
			name: "already current manifest is returned unchanged",
			goMod: `module github.com/grafana/ai-sdk/middleware/logger

go 1.27.3

require github.com/grafana/ai-sdk v0.1.0-alpha.4
`,
			want: `module github.com/grafana/ai-sdk/middleware/logger

go 1.27.3

require github.com/grafana/ai-sdk v0.1.0-alpha.4
`,
		},
		{
			name: "untagged prerequisite",
			goMod: `module github.com/grafana/ai-sdk/providers/azure

go 1.27.3

require github.com/grafana/ai-sdk/providers/anthropic v0.0.0-20260825181821-1f7f7ef4a4c8
`,
			wantErr: ErrUntaggedPrerequisite,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed, err := plan.RewriteRequirements("go.mod", []byte(tc.goMod))
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantChanged, changed)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestPlanRewriteRequirements_PreservesRepositoryManifests(t *testing.T) {
	repo := repositoryRoot(t)
	for _, dir := range []string{".", "providers/bedrock", GatewayDir} {
		t.Run(dir, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(repo, dir, "go.mod"))
			require.NoError(t, err)
			plan := Plan{Released: map[string]string{}, Latest: map[string]string{}}
			identity, err := currentRequirements(content)
			require.NoError(t, err)
			plan.Latest = identity
			got, changed, err := plan.RewriteRequirements(dir+"/go.mod", content)
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Equal(t, string(content), string(got))
		})
	}
}

func currentRequirements(goMod []byte) (map[string]string, error) {
	file, err := modfile.Parse("go.mod", goMod, nil)
	if err != nil {
		return nil, err
	}
	versions := map[string]string{}
	for _, requirement := range file.Require {
		if firstParty(requirement.Mod.Path) {
			versions[requirement.Mod.Path] = requirement.Mod.Version
		}
	}
	return versions, nil
}

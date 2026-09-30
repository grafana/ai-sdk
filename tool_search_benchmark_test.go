package aisdk

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkToolSearch_PrepareActiveTools(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			tools := make(ToolSet, size)
			active := make([]string, size)
			for i := range size {
				active[i] = "tool" + strconv.Itoa(i)
				tools[active[i]] = Tool{DeferLoading: true}
			}
			state, err := newToolSearchState(tools, nil)
			require.NoError(b, err)
			b.ReportAllocs()
			for b.Loop() {
				state.prepare(tools, active, true, nil)
			}
		})
	}
}

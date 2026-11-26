package aisdk

import (
	"slices"

	"github.com/grafana/ai-sdk/provider"
)

// PruneReasoningMode controls explicit pruning of assistant reasoning parts.
type PruneReasoningMode string

const (
	// PruneReasoningNone leaves reasoning unchanged by the explicit reasoning filter.
	PruneReasoningNone PruneReasoningMode = "none"
	// PruneReasoningAll removes all assistant reasoning text and files.
	PruneReasoningAll PruneReasoningMode = "all"
	// PruneReasoningBeforeLastMessage removes assistant reasoning text and files
	// before the final input message, regardless of that message's role.
	PruneReasoningBeforeLastMessage PruneReasoningMode = "before-last-message"
)

// PruneOption configures [PruneMessages].
type PruneOption func(*pruneConfig)

type pruneConfig struct {
	reasoning PruneReasoningMode
	toolRules []pruneToolRule
	keepEmpty bool
}

type pruneToolRule struct {
	keepLast int
	tools    []string
}

// WithPruneReasoning sets how assistant reasoning text and files are pruned.
// The last setting wins. Tool pruning may separately remove reasoning that
// was followed only by removed tool parts.
func WithPruneReasoning(mode PruneReasoningMode) PruneOption {
	return func(cfg *pruneConfig) { cfg.reasoning = mode }
}

// WithPruneToolCalls appends a rule to prune tool calls, results, and approvals.
// Zero protects no messages, a positive count protects that many trailing
// input messages (including empty ones), and a negative count disables the rule.
// Linked calls and results needed by the protected suffix, including by a
// retained approval, are also protected.
// Omit tool names (or pass a nil slice) to select all tools. A non-nil empty
// slice selects no known tool names. Unresolved approvals outside the suffix
// are pruned even with a name list. Rules run in the order supplied.
func WithPruneToolCalls(keepLastMessages int, tools ...string) PruneOption {
	rule := pruneToolRule{keepLast: keepLastMessages, tools: slices.Clone(tools)}
	return func(cfg *pruneConfig) { cfg.toolRules = append(cfg.toolRules, rule) }
}

// WithPruneEmptyMessages controls whether zero-part messages are removed after
// pruning. The default is true, including for messages that were already empty.
func WithPruneEmptyMessages(remove bool) PruneOption {
	return func(cfg *pruneConfig) { cfg.keepEmpty = !remove }
}

// PruneMessages returns model history filtered according to opts. With no
// options it retains every content part and discards zero-part messages. It
// runs only when explicitly called. Call it before generation handles tool
// approvals so that approval requests are still available for correlation.
// The returned message and content slices are independent of the input;
// nested values, including provider options and payloads, are shared.
func PruneMessages(messages []provider.Message, opts ...PruneOption) []provider.Message {
	var cfg pruneConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	pruned := slices.Clone(messages)
	for i := range pruned {
		msg := &pruned[i]
		msg.Content = slices.Clone(msg.Content)
		if msg.Role == provider.RoleAssistant &&
			(cfg.reasoning == PruneReasoningAll || cfg.reasoning == PruneReasoningBeforeLastMessage && i < len(pruned)-1) {
			msg.Content = slices.DeleteFunc(msg.Content, isPrunableReasoning)
		}
	}
	for _, rule := range cfg.toolRules {
		pruneToolHistory(pruned, rule)
	}
	if !cfg.keepEmpty {
		pruned = slices.DeleteFunc(pruned, func(msg provider.Message) bool { return len(msg.Content) == 0 })
	}
	return pruned
}

func isPrunableReasoning(part provider.ContentPart) bool {
	return part.Type == provider.ContentPartTypeReasoning || part.Type == provider.ContentPartTypeReasoningFile
}

func pruneToolHistory(messages []provider.Message, rule pruneToolRule) {
	if rule.keepLast < 0 || rule.keepLast >= len(messages) {
		return
	}
	end := len(messages) - rule.keepLast
	keptCalls := make(map[string]struct{})
	keptApprovals := make(map[string]struct{})
	callNames := make(map[string]string)
	approvalCalls := make(map[string]string)
	for i, msg := range messages {
		if msg.Role != provider.RoleAssistant && msg.Role != provider.RoleTool {
			continue
		}
		for _, part := range msg.Content {
			switch part.Type {
			case provider.ContentPartTypeToolCall, provider.ContentPartTypeToolResult:
				callNames[part.ToolCallID] = part.ToolName
				if i >= end {
					keptCalls[part.ToolCallID] = struct{}{}
				}
			case provider.ContentPartTypeToolApprovalRequest:
				approvalCalls[part.ApprovalID] = part.ToolCallID
				fallthrough
			case provider.ContentPartTypeToolApprovalResponse:
				if i >= end {
					keptApprovals[part.ApprovalID] = struct{}{}
				}
			}
		}
	}
	for id := range keptApprovals {
		if call, ok := approvalCalls[id]; ok {
			keptCalls[call] = struct{}{}
		}
	}

	keepPart := func(part provider.ContentPart) bool {
		switch part.Type {
		case provider.ContentPartTypeToolCall, provider.ContentPartTypeToolResult:
			_, kept := keptCalls[part.ToolCallID]
			return kept || rule.tools != nil && !slices.Contains(rule.tools, part.ToolName)
		case provider.ContentPartTypeToolApprovalRequest, provider.ContentPartTypeToolApprovalResponse:
			if _, kept := keptApprovals[part.ApprovalID]; kept {
				return true
			}
			call, linked := approvalCalls[part.ApprovalID]
			name, named := callNames[call]
			return linked && named && rule.tools != nil && !slices.Contains(rule.tools, name)
		default:
			return true
		}
	}
	for i := range messages[:end] {
		msg := &messages[i]
		switch msg.Role {
		case provider.RoleAssistant:
			msg.Content = pruneAssistantToolParts(msg.Content, keepPart)
		case provider.RoleTool:
			msg.Content = slices.DeleteFunc(msg.Content, func(part provider.ContentPart) bool { return !keepPart(part) })
		}
	}
}

func pruneAssistantToolParts(parts []provider.ContentPart, keepPart func(provider.ContentPart) bool) []provider.ContentPart {
	out := parts[:0]
	for i := 0; i < len(parts); {
		start := i
		for i < len(parts) && isPrunableReasoning(parts[i]) {
			i++
		}
		endReasoning := i
		outBefore := len(out)
		out = append(out, parts[start:endReasoning]...)
		outAfterReasoning := len(out)
		for i < len(parts) && !isPrunableReasoning(parts[i]) {
			if keepPart(parts[i]) {
				out = append(out, parts[i])
			}
			i++
		}
		if i > endReasoning && len(out) == outAfterReasoning {
			out = out[:outBefore]
		}
	}
	clear(parts[len(out):])
	return out
}

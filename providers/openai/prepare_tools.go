package openai

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// Provider tool ids recognized by the OpenAI Responses provider.
const (
	toolIDWebSearch        = "openai.web_search"
	toolIDWebSearchPreview = "openai.web_search_preview"
	toolIDCodeInterpreter  = "openai.code_interpreter"
	toolIDFileSearch       = "openai.file_search"
	toolIDImageGeneration  = "openai.image_generation"
	toolIDLocalShell       = "openai.local_shell"
	toolIDShell            = "openai.shell"
	toolIDApplyPatch       = "openai.apply_patch"
	toolIDComputer         = "openai.computer"
	toolIDMCP              = "openai.mcp"
	toolIDToolSearch       = "openai.tool_search"
	toolIDProgrammatic     = "openai.programmatic_tool_calling"
	toolIDCustom           = "openai.custom"
)

type shellNetworkPolicyArg struct {
	Type           string   `json:"type"`
	AllowedDomains []string `json:"allowedDomains"`
	DomainSecrets  []struct {
		Domain string `json:"domain"`
		Name   string `json:"name"`
		Value  string `json:"value"`
	} `json:"domainSecrets"`
}

// prepareTools converts CallOptions tools and tool choice into the Responses
// request, returning warnings for unsupported tools. It also records tool
// presence flags on the buildResult for include auto-population.
func prepareTools(body *responses.ResponseNewParams, opts provider.CallOptions, popts OpenAIResponsesOptions, br *buildResult) ([]provider.Warning, error) {
	var warnings []provider.Warning

	if len(opts.Tools) == 0 {
		return nil, nil
	}

	var tools []responses.ToolUnionParam
	namespaceTools := map[string]*responses.NamespaceToolParam{}

	for _, t := range opts.Tools {
		switch t.Type {
		case provider.ToolTypeFunction:
			openaiOptions, err := toolOptions(t)
			if err != nil {
				return nil, err
			}
			if openaiOptions.Namespace != nil {
				namespace := openaiOptions.Namespace
				namespaceTool := namespaceTools[namespace.Name]
				if namespaceTool == nil {
					namespaceTool = &responses.NamespaceToolParam{
						Name:        namespace.Name,
						Description: namespace.Description,
					}
					namespaceTools[namespace.Name] = namespaceTool
					tools = append(tools, responses.ToolUnionParam{OfNamespace: namespaceTool})
				} else if namespaceTool.Description != namespace.Description {
					return nil, fmt.Errorf("openai: conflicting descriptions for OpenAI tool namespace %q", namespace.Name)
				}
				namespaceTool.Tools = append(namespaceTool.Tools, namespaceFunctionTool(t, openaiOptions))
				continue
			}
			tools = append(tools, functionTool(t, openaiOptions))

		case provider.ToolTypeProvider:
			tool, w, ok, err := providerTool(t, br)
			if err != nil {
				return nil, err
			}
			if !ok {
				warnings = append(warnings, w...)
				continue
			}
			warnings = append(warnings, w...)
			tools = append(tools, tool)

		default:
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "tool",
				Details: "unsupported tool type",
			})
		}
	}

	if len(tools) > 0 {
		body.Tools = tools
	}

	applyToolChoice(body, opts.ToolChoice, popts, opts.Tools, br.toolNameMapping)
	return warnings, nil
}

func functionTool(t provider.Tool, options OpenAIToolOptions) responses.ToolUnionParam {
	fn := functionToolParam(t, options)
	return responses.ToolUnionParam{OfFunction: &fn}
}

func functionToolParam(t provider.Tool, options OpenAIToolOptions) responses.FunctionToolParam {
	var params map[string]any
	if len(t.InputSchema) > 0 {
		_ = json.Unmarshal(t.InputSchema, &params)
	}
	fn := responses.FunctionToolParam{
		Name:       t.Name,
		Parameters: params,
	}
	if t.Strict != nil {
		fn.Strict = param.NewOpt(*t.Strict)
	}
	if t.Description != "" {
		fn.Description = param.NewOpt(t.Description)
	}
	if options.DeferLoading != nil {
		fn.DeferLoading = param.NewOpt(*options.DeferLoading)
	}
	for _, caller := range options.AllowedCallers {
		fn.AllowedCallers = append(fn.AllowedCallers, string(caller))
	}
	if len(options.OutputSchema) > 0 {
		_ = json.Unmarshal(options.OutputSchema, &fn.OutputSchema)
	}
	return fn
}

func namespaceFunctionTool(t provider.Tool, options OpenAIToolOptions) responses.NamespaceToolToolUnionParam {
	fn := functionToolParam(t, options)
	namespaceFn := responses.NamespaceToolToolFunctionParam{
		Name:           fn.Name,
		Parameters:     fn.Parameters,
		Strict:         fn.Strict,
		Description:    fn.Description,
		DeferLoading:   fn.DeferLoading,
		AllowedCallers: fn.AllowedCallers,
		OutputSchema:   fn.OutputSchema,
	}
	return responses.NamespaceToolToolUnionParam{OfFunction: &namespaceFn}
}

func providerTool(t provider.Tool, br *buildResult) (responses.ToolUnionParam, []provider.Warning, bool, error) {
	switch t.ID {
	case toolIDWebSearch:
		if br.webSearchToolName == "" {
			br.webSearchToolName = t.Name
		}
		br.hasWebSearchTool = true
		return webSearchTool(t), nil, true, nil

	case toolIDWebSearchPreview:
		if br.webSearchToolName == "" {
			br.webSearchToolName = t.Name
		}
		br.hasWebSearchTool = true
		return responses.ToolUnionParam{OfWebSearchPreview: webSearchPreviewTool(t)}, nil, true, nil

	case toolIDCodeInterpreter:
		br.hasCodeInterpreterTool = true
		return codeInterpreterTool(t), nil, true, nil

	case toolIDFileSearch:
		return fileSearchTool(t), nil, true, nil

	case toolIDMCP:
		return responses.ToolUnionParam{OfMcp: mcpTool(t)}, nil, true, nil

	case toolIDCustom:
		tool, err := customTool(t)
		if err != nil {
			return responses.ToolUnionParam{}, nil, false, err
		}
		return responses.ToolUnionParam{OfCustom: tool}, nil, true, nil

	case toolIDImageGeneration:
		return responses.ToolUnionParam{OfImageGeneration: imageGenerationTool(t)}, nil, true, nil

	case toolIDLocalShell:
		return responses.ToolUnionParam{OfLocalShell: &responses.ToolLocalShellParam{}}, nil, true, nil

	case toolIDShell:
		br.isShellProviderExecuted = shellEnvironmentType(t.Args) == "containerAuto" || shellEnvironmentType(t.Args) == "containerReference"
		return responses.ToolUnionParam{OfShell: shellTool(t)}, nil, true, nil

	case toolIDApplyPatch:
		return responses.ToolUnionParam{OfApplyPatch: &responses.ApplyPatchToolParam{}}, nil, true, nil

	case toolIDComputer:
		br.hasComputerTool = true
		return responses.ToolUnionParam{OfComputer: &responses.ComputerToolParam{}}, nil, true, nil

	case toolIDToolSearch:
		return responses.ToolUnionParam{OfToolSearch: toolSearchTool(t)}, nil, true, nil

	case toolIDProgrammatic:
		tool := responses.NewToolProgrammaticToolCallingParam()
		return responses.ToolUnionParam{OfProgrammaticToolCalling: &tool}, nil, true, nil

	default:
		return responses.ToolUnionParam{}, []provider.Warning{{
			Type:    provider.WarnUnsupported,
			Feature: "tool",
			Details: "unsupported provider tool: " + t.ID,
		}}, false, nil
	}
}

func toolOptions(t provider.Tool) (OpenAIToolOptions, error) {
	options, ok, err := provider.ResolveOption[OpenAIToolOptions](t.ProviderOptions, "openai")
	if err != nil || !ok {
		return OpenAIToolOptions{}, err
	}
	return options, nil
}

func webSearchTool(t provider.Tool) responses.ToolUnionParam {
	ws := responses.WebSearchToolParam{Type: "web_search"}
	var explicitFilters map[string]any
	if size := stringArg(t.Args, "searchContextSize"); size != "" {
		ws.SearchContextSize = responses.WebSearchToolSearchContextSize(size)
	}
	if rawFilters, ok := rawArg(t.Args, "filters"); ok {
		var values map[string]json.RawMessage
		if json.Unmarshal(rawFilters, &values) == nil {
			allowedDomains, allowedSet := stringSliceArgPresent(values, "allowedDomains")
			blockedDomains, blockedSet := stringSliceArgPresent(values, "blockedDomains")
			explicitFilters = map[string]any{}
			if allowedSet {
				explicitFilters["allowed_domains"] = allowedDomains
			}
			if blockedSet {
				explicitFilters["blocked_domains"] = blockedDomains
			}
		}
	}
	if loc, ok := userLocationArg[responses.WebSearchToolUserLocationParam](t.Args); ok {
		ws.UserLocation = loc
	}
	if externalWebAccess, ok := boolArg(t.Args, "externalWebAccess"); ok {
		ws.SetExtraFields(map[string]any{"external_web_access": externalWebAccess})
	}
	if explicitFilters != nil {
		data, _ := json.Marshal(ws)
		var fields map[string]any
		_ = json.Unmarshal(data, &fields)
		fields["filters"] = explicitFilters
		return param.Override[responses.ToolUnionParam](fields)
	}
	return responses.ToolUnionParam{OfWebSearch: &ws}
}

func webSearchPreviewTool(t provider.Tool) *responses.WebSearchPreviewToolParam {
	ws := responses.WebSearchPreviewToolParam{Type: "web_search_preview"}
	if size := stringArg(t.Args, "searchContextSize"); size != "" {
		ws.SearchContextSize = responses.WebSearchPreviewToolSearchContextSize(size)
	}
	if loc, ok := userLocationArg[responses.WebSearchPreviewToolUserLocationParam](t.Args); ok {
		ws.UserLocation = loc
	}
	return &ws
}

func fileSearchTool(t provider.Tool) responses.ToolUnionParam {
	fs := responses.FileSearchToolParam{
		VectorStoreIDs: stringSliceArg(t.Args, "vectorStoreIds"),
	}
	if maxResults, ok := int64Arg(t.Args, "maxNumResults"); ok {
		fs.MaxNumResults = param.NewOpt(maxResults)
	}
	if ranking, ok := fileSearchRankingArg(t.Args); ok {
		fs.RankingOptions = ranking
	}
	if raw, ok := rawArg(t.Args, "filters"); ok {
		fs.Filters = param.Override[responses.FileSearchToolFiltersUnionParam](raw)
	}
	return responses.ToolUnionParam{OfFileSearch: &fs}
}

func codeInterpreterTool(t provider.Tool) responses.ToolUnionParam {
	var tool responses.ToolCodeInterpreterParam
	raw, ok := rawArg(t.Args, "container")
	if !ok {
		tool.Container.OfCodeInterpreterToolAuto = &responses.ToolCodeInterpreterContainerCodeInterpreterContainerAutoParam{}
		return responses.ToolUnionParam{OfCodeInterpreter: &tool}
	}
	var containerID string
	if err := json.Unmarshal(raw, &containerID); err == nil {
		tool.Container.OfString = param.NewOpt(containerID)
		return responses.ToolUnionParam{OfCodeInterpreter: &tool}
	}
	var container struct {
		FileIDs []string `json:"fileIds"`
	}
	_ = json.Unmarshal(raw, &container)
	tool.Container.OfCodeInterpreterToolAuto = &responses.ToolCodeInterpreterContainerCodeInterpreterContainerAutoParam{
		FileIDs: container.FileIDs,
	}
	return responses.ToolUnionParam{OfCodeInterpreter: &tool}
}

func imageGenerationTool(t provider.Tool) *responses.ToolImageGenerationParam {
	ig := responses.ToolImageGenerationParam{}
	if v := stringArg(t.Args, "background"); v != "" {
		ig.Background = v
	}
	if v := stringArg(t.Args, "inputFidelity"); v != "" {
		ig.InputFidelity = v
	}
	if v := stringArg(t.Args, "model"); v != "" {
		ig.Model = v
	}
	if v := stringArg(t.Args, "moderation"); v != "" {
		ig.Moderation = v
	}
	if v := stringArg(t.Args, "outputFormat"); v != "" {
		ig.OutputFormat = v
	}
	if v := stringArg(t.Args, "quality"); v != "" {
		ig.Quality = v
	}
	if v := stringArg(t.Args, "size"); v != "" {
		ig.Size = v
	}
	if v, ok := int64Arg(t.Args, "outputCompression"); ok {
		ig.OutputCompression = param.NewOpt(v)
	}
	if v, ok := int64Arg(t.Args, "partialImages"); ok {
		ig.PartialImages = param.NewOpt(v)
	}
	if mask, ok := imageGenerationMaskArg(t.Args); ok {
		ig.InputImageMask = mask
	}
	return &ig
}

func customTool(t provider.Tool) (*responses.CustomToolParam, error) {
	options, err := toolOptions(t)
	if err != nil {
		return nil, err
	}
	custom := responses.CustomToolParam{Name: t.Name}
	if desc := stringArg(t.Args, "description"); desc != "" {
		custom.Description = param.NewOpt(desc)
	}
	if raw, ok := rawArg(t.Args, "format"); ok {
		custom.Format = param.Override[shared.CustomToolInputFormatUnionParam](raw)
	}
	if options.DeferLoading != nil {
		custom.DeferLoading = param.NewOpt(*options.DeferLoading)
	}
	return &custom, nil
}

func mcpTool(t provider.Tool) *responses.ToolMcpParam {
	mcp := responses.ToolMcpParam{ServerLabel: stringArg(t.Args, "serverLabel")}
	if url := stringArg(t.Args, "serverUrl"); url != "" {
		mcp.ServerURL = param.NewOpt(url)
	}
	if auth := stringArg(t.Args, "authorization"); auth != "" {
		mcp.Authorization = param.NewOpt(auth)
	}
	if connectorID := stringArg(t.Args, "connectorId"); connectorID != "" {
		mcp.ConnectorID = connectorID
	}
	if desc := stringArg(t.Args, "serverDescription"); desc != "" {
		mcp.ServerDescription = param.NewOpt(desc)
	}
	if headers := stringMapArg(t.Args, "headers"); len(headers) > 0 {
		mcp.Headers = headers
	}
	if allowedTools, ok := mcpAllowedToolsArg(t.Args); ok {
		mcp.AllowedTools = allowedTools
	}
	if requireApproval, ok := mcpRequireApprovalArg(t.Args); ok {
		mcp.RequireApproval = requireApproval
	} else {
		mcp.RequireApproval = responses.ToolMcpRequireApprovalUnionParam{
			OfMcpToolApprovalSetting: param.NewOpt("never"),
		}
	}
	return &mcp
}

func shellTool(t provider.Tool) *responses.FunctionShellToolParam {
	sh := responses.FunctionShellToolParam{}
	if env, ok := shellEnvironmentArg(t.Args); ok {
		sh.Environment = env
	}
	return &sh
}

func toolSearchTool(t provider.Tool) *responses.ToolSearchToolParam {
	ts := responses.ToolSearchToolParam{}
	if desc := stringArg(t.Args, "description"); desc != "" {
		ts.Description = param.NewOpt(desc)
	}
	if execution := stringArg(t.Args, "execution"); execution != "" {
		ts.Execution = responses.ToolSearchToolExecution(execution)
	}
	if raw, ok := rawArg(t.Args, "parameters"); ok {
		var params map[string]any
		if err := json.Unmarshal(raw, &params); err == nil {
			ts.Parameters = params
		}
	}
	return &ts
}

// applyToolChoice resolves the tool choice, honoring an allowedTools override.
func applyToolChoice(body *responses.ResponseNewParams, tc *provider.ToolChoice, popts OpenAIResponsesOptions, tools []provider.Tool, mapping toolNameMapping) {
	// allowedTools overrides the request tool choice entirely.
	if popts.AllowedTools != nil && len(popts.AllowedTools.ToolNames) > 0 {
		var fns []map[string]any
		for _, name := range popts.AllowedTools.ToolNames {
			fns = append(fns, map[string]any{"type": "function", "name": mapping.toProviderToolName(name)})
		}
		mode := responses.ToolChoiceAllowedMode("auto")
		if popts.AllowedTools.Mode != "" {
			mode = responses.ToolChoiceAllowedMode(popts.AllowedTools.Mode)
		}
		body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfAllowedTools: &responses.ToolChoiceAllowedParam{
				Mode:  mode,
				Tools: fns,
			},
		}
		return
	}

	if tc == nil {
		return
	}

	switch tc.Type {
	case provider.ToolChoiceAuto:
		body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto)}
	case provider.ToolChoiceNone:
		body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsNone)}
	case provider.ToolChoiceRequired:
		body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired)}
	case provider.ToolChoiceTool:
		if customProviderToolChoice(tc.ToolName, tools) {
			body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfCustomTool: &responses.ToolChoiceCustomParam{Name: tc.ToolName},
			}
			return
		}
		resolvedToolName := mapping.toProviderToolName(tc.ToolName)
		if hostedToolType := hostedToolChoiceType(resolvedToolName); hostedToolType != "" {
			switch hostedToolType {
			case "programmatic_tool_calling":
				toolChoice := responses.NewResponseNewParamsToolChoiceSpecificProgrammaticToolCallingParam()
				body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfResponseNewsToolChoiceSpecificProgrammaticToolCallingParam: &toolChoice}
			case "apply_patch":
				toolChoice := responses.NewToolChoiceApplyPatchParam()
				body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfSpecificApplyPatchToolChoice: &toolChoice}
			case "shell":
				toolChoice := responses.NewToolChoiceShellParam()
				body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfSpecificShellToolChoice: &toolChoice}
			default:
				body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
					OfHostedTool: &responses.ToolChoiceTypesParam{Type: responses.ToolChoiceTypesType(hostedToolType)},
				}
			}
		} else {
			body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfFunctionTool: &responses.ToolChoiceFunctionParam{Name: resolvedToolName},
			}
		}
	}
}

// hostedToolChoiceType returns the hosted tool type for a named built-in tool,
// or "" if the name refers to a function tool.
func hostedToolChoiceType(name string) string {
	for _, providerToolName := range providerToolNames {
		if providerToolName == name {
			return providerToolName
		}
	}
	return ""
}

func customProviderToolChoice(name string, tools []provider.Tool) bool {
	for _, t := range tools {
		if t.Type == provider.ToolTypeProvider && t.ID == toolIDCustom && t.Name == name {
			return true
		}
	}
	return false
}

func rawArg(args map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	if args == nil {
		return nil, false
	}
	raw, ok := args[key]
	return raw, ok
}

func stringArg(args map[string]json.RawMessage, key string) string {
	if args == nil {
		return ""
	}
	raw, ok := args[key]
	if !ok {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func int64Arg(args map[string]json.RawMessage, key string) (int64, bool) {
	if args == nil {
		return 0, false
	}
	raw, ok := args[key]
	if !ok {
		return 0, false
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int64(f), true
	}
	return 0, false
}

func boolArg(args map[string]json.RawMessage, key string) (bool, bool) {
	if args == nil {
		return false, false
	}
	raw, ok := args[key]
	if !ok {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

func float64Arg(args map[string]json.RawMessage, key string) (float64, bool) {
	if args == nil {
		return 0, false
	}
	raw, ok := args[key]
	if !ok {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	return f, true
}

func stringMapArg(args map[string]json.RawMessage, key string) map[string]string {
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok {
		return nil
	}
	var m map[string]string
	_ = json.Unmarshal(raw, &m)
	return m
}

func fileSearchRankingArg(args map[string]json.RawMessage) (responses.FileSearchToolRankingOptionsParam, bool) {
	raw, ok := rawArg(args, "ranking")
	if !ok {
		return responses.FileSearchToolRankingOptionsParam{}, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return responses.FileSearchToolRankingOptionsParam{}, false
	}
	ranking := responses.FileSearchToolRankingOptionsParam{Ranker: stringArg(obj, "ranker")}
	if score, ok := float64Arg(obj, "scoreThreshold"); ok {
		ranking.ScoreThreshold = param.NewOpt(score)
	}
	return ranking, true
}

func userLocationArg[T any](args map[string]json.RawMessage) (T, bool) {
	var zero T
	raw, ok := rawArg(args, "userLocation")
	if !ok {
		return zero, false
	}
	var loc T
	if err := json.Unmarshal(raw, &loc); err != nil {
		return zero, false
	}
	return loc, true
}

func imageGenerationMaskArg(args map[string]json.RawMessage) (responses.ToolImageGenerationInputImageMaskParam, bool) {
	raw, ok := rawArg(args, "inputImageMask")
	if !ok {
		return responses.ToolImageGenerationInputImageMaskParam{}, false
	}
	var mask struct {
		FileID   string `json:"fileId"`
		ImageURL string `json:"imageUrl"`
	}
	if err := json.Unmarshal(raw, &mask); err != nil {
		return responses.ToolImageGenerationInputImageMaskParam{}, false
	}
	result := responses.ToolImageGenerationInputImageMaskParam{}
	if mask.FileID != "" {
		result.FileID = param.NewOpt(mask.FileID)
	}
	if mask.ImageURL != "" {
		result.ImageURL = param.NewOpt(mask.ImageURL)
	}
	return result, true
}

func mcpAllowedToolsArg(args map[string]json.RawMessage) (responses.ToolMcpAllowedToolsUnionParam, bool) {
	raw, ok := rawArg(args, "allowedTools")
	if !ok {
		return responses.ToolMcpAllowedToolsUnionParam{}, false
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return responses.ToolMcpAllowedToolsUnionParam{OfMcpAllowedTools: names}, true
	}
	var filter struct {
		ReadOnly  *bool    `json:"readOnly"`
		ToolNames []string `json:"toolNames"`
	}
	if err := json.Unmarshal(raw, &filter); err != nil {
		return responses.ToolMcpAllowedToolsUnionParam{}, false
	}
	paramFilter := responses.ToolMcpAllowedToolsMcpToolFilterParam{ToolNames: filter.ToolNames}
	if filter.ReadOnly != nil {
		paramFilter.ReadOnly = param.NewOpt(*filter.ReadOnly)
	}
	return responses.ToolMcpAllowedToolsUnionParam{OfMcpToolFilter: &paramFilter}, true
}

func mcpRequireApprovalArg(args map[string]json.RawMessage) (responses.ToolMcpRequireApprovalUnionParam, bool) {
	raw, ok := rawArg(args, "requireApproval")
	if !ok {
		return responses.ToolMcpRequireApprovalUnionParam{}, false
	}
	var setting string
	if err := json.Unmarshal(raw, &setting); err == nil {
		return responses.ToolMcpRequireApprovalUnionParam{OfMcpToolApprovalSetting: param.NewOpt(setting)}, true
	}
	var filter struct {
		Always *struct {
			ReadOnly  *bool    `json:"readOnly"`
			ToolNames []string `json:"toolNames"`
		} `json:"always"`
		Never *struct {
			ReadOnly  *bool    `json:"readOnly"`
			ToolNames []string `json:"toolNames"`
		} `json:"never"`
	}
	if err := json.Unmarshal(raw, &filter); err != nil {
		return responses.ToolMcpRequireApprovalUnionParam{}, false
	}
	paramFilter := responses.ToolMcpRequireApprovalMcpToolApprovalFilterParam{}
	if filter.Always != nil {
		always := responses.ToolMcpRequireApprovalMcpToolApprovalFilterAlwaysParam{ToolNames: filter.Always.ToolNames}
		if filter.Always.ReadOnly != nil {
			always.ReadOnly = param.NewOpt(*filter.Always.ReadOnly)
		}
		paramFilter.Always = always
	}
	if filter.Never != nil {
		never := responses.ToolMcpRequireApprovalMcpToolApprovalFilterNeverParam{ToolNames: filter.Never.ToolNames}
		if filter.Never.ReadOnly != nil {
			never.ReadOnly = param.NewOpt(*filter.Never.ReadOnly)
		}
		paramFilter.Never = never
	}
	return responses.ToolMcpRequireApprovalUnionParam{OfMcpToolApprovalFilter: &paramFilter}, true
}

func shellEnvironmentType(args map[string]json.RawMessage) string {
	raw, ok := rawArg(args, "environment")
	if !ok {
		return ""
	}
	var env struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &env)
	return env.Type
}

func shellEnvironmentArg(args map[string]json.RawMessage) (responses.FunctionShellToolEnvironmentUnionParam, bool) {
	raw, ok := rawArg(args, "environment")
	if !ok {
		return responses.FunctionShellToolEnvironmentUnionParam{}, false
	}
	var env struct {
		Type          string                      `json:"type"`
		FileIDs       []string                    `json:"fileIds"`
		MemoryLimit   string                      `json:"memoryLimit"`
		ContainerID   string                      `json:"containerId"`
		NetworkPolicy *shellNetworkPolicyArg      `json:"networkPolicy"`
		Skills        []responses.LocalSkillParam `json:"skills"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return responses.FunctionShellToolEnvironmentUnionParam{}, false
	}

	switch env.Type {
	case "containerAuto":
		container := responses.ContainerAutoParam{
			FileIDs:     env.FileIDs,
			MemoryLimit: responses.ContainerAutoMemoryLimit(env.MemoryLimit),
		}
		if env.NetworkPolicy != nil {
			container.NetworkPolicy = containerNetworkPolicyArg(*env.NetworkPolicy)
		}
		return responses.FunctionShellToolEnvironmentUnionParam{OfContainerAuto: &container}, true
	case "containerReference":
		return responses.FunctionShellToolEnvironmentUnionParam{
			OfContainerReference: &responses.ContainerReferenceParam{ContainerID: env.ContainerID},
		}, true
	default:
		return responses.FunctionShellToolEnvironmentUnionParam{
			OfLocal: &responses.LocalEnvironmentParam{Skills: env.Skills},
		}, true
	}
}

func containerNetworkPolicyArg(policy shellNetworkPolicyArg) responses.ContainerAutoNetworkPolicyUnionParam {
	if policy.Type == "disabled" {
		disabled := responses.NewContainerNetworkPolicyDisabledParam()
		return responses.ContainerAutoNetworkPolicyUnionParam{OfDisabled: &disabled}
	}
	allowlist := responses.ContainerNetworkPolicyAllowlistParam{AllowedDomains: policy.AllowedDomains}
	for _, secret := range policy.DomainSecrets {
		allowlist.DomainSecrets = append(allowlist.DomainSecrets, responses.ContainerNetworkPolicyDomainSecretParam{
			Domain: secret.Domain,
			Name:   secret.Name,
			Value:  secret.Value,
		})
	}
	return responses.ContainerAutoNetworkPolicyUnionParam{OfAllowlist: &allowlist}
}

func stringSliceArg(args map[string]json.RawMessage, key string) []string {
	values, _ := stringSliceArgPresent(args, key)
	return values
}

func stringSliceArgPresent(args map[string]json.RawMessage, key string) ([]string, bool) {
	if args == nil {
		return nil, false
	}
	raw, ok := args[key]
	if !ok {
		return nil, false
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, false
	}
	if values == nil {
		values = []string{}
	}
	return values, true
}

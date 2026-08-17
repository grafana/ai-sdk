package openai

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

type inputConversionContext struct {
	store                   bool
	providerOptionsName     string
	hasConversation         bool
	hasPreviousResponseID   bool
	toolNameMapping         toolNameMapping
	hasLocalShellTool       bool
	hasShellTool            bool
	hasApplyPatchTool       bool
	hasComputerTool         bool
	customProviderToolNames map[string]struct{}
	outputSchemaToolNames   map[string]struct{}
	processedApprovalIDs    map[string]struct{}
}

func newInputConversionContext(tools []provider.Tool, mapping toolNameMapping, store bool, providerOptionsName string, hasConversation, hasPreviousResponseID bool) inputConversionContext {
	ctx := inputConversionContext{
		store:                   store,
		providerOptionsName:     providerOptionsName,
		hasConversation:         hasConversation,
		hasPreviousResponseID:   hasPreviousResponseID,
		toolNameMapping:         mapping,
		customProviderToolNames: make(map[string]struct{}),
		outputSchemaToolNames:   make(map[string]struct{}),
		processedApprovalIDs:    make(map[string]struct{}),
	}
	for _, tool := range tools {
		if tool.Type == provider.ToolTypeFunction {
			options, err := toolOptions(tool)
			if err == nil && len(options.OutputSchema) > 0 && !isJSONNull(options.OutputSchema) {
				ctx.outputSchemaToolNames[tool.Name] = struct{}{}
			}
			continue
		}
		if tool.Type != provider.ToolTypeProvider {
			continue
		}
		switch tool.ID {
		case toolIDLocalShell:
			ctx.hasLocalShellTool = true
		case toolIDShell:
			ctx.hasShellTool = true
		case toolIDApplyPatch:
			ctx.hasApplyPatchTool = true
		case toolIDComputer:
			ctx.hasComputerTool = true
		case toolIDCustom:
			ctx.customProviderToolNames[tool.Name] = struct{}{}
		}
	}
	return ctx
}

func (c inputConversionContext) isCustomProviderTool(name string) bool {
	_, ok := c.customProviderToolNames[name]
	return ok
}

func (c inputConversionContext) hasOutputSchema(name string) bool {
	_, ok := c.outputSchemaToolNames[name]
	return ok
}

func (c inputConversionContext) partOptions(part provider.ContentPart) OpenAIPartOptions {
	return openAIPartOptionsFor(part.ProviderOptions, c.providerOptionsName)
}

func (c inputConversionContext) outputOptions(output *provider.ToolResultOutput) OpenAIPartOptions {
	if output == nil {
		return OpenAIPartOptions{}
	}
	return openAIPartOptionsFor(output.ProviderOptions, c.providerOptionsName)
}

func (c inputConversionContext) contentOptions(content provider.ToolResultContentValue) OpenAIPartOptions {
	return openAIPartOptionsFor(content.ProviderOptions, c.providerOptionsName)
}

func convertAssistantToolCall(part provider.ContentPart, ctx inputConversionContext) (*responses.ResponseInputItemUnionParam, error) {
	po := ctx.partOptions(part)
	if ctx.hasConversation && po.ItemID != "" {
		return nil, nil
	}

	toolName := ctx.toolNameMapping.toProviderToolName(part.ToolName)
	if toolName == "tool_search" {
		if ctx.store && po.ItemID != "" {
			item := itemReference(po.ItemID)
			return &item, nil
		}
		item, err := toolSearchCallItem(part, po.ItemID)
		return item, err
	}

	if toolName == "programmatic_tool_calling" {
		if ctx.store && po.ItemID != "" {
			item := itemReference(po.ItemID)
			return &item, nil
		}
		item, err := programmaticToolCallItem(part, po.ItemID)
		return item, err
	}

	if ctx.hasComputerTool && toolName == "computer" {
		if ctx.store && po.ItemID != "" {
			if ctx.hasPreviousResponseID {
				return nil, nil
			}
			item := itemReference(po.ItemID)
			return &item, nil
		}
		item, err := computerCallInputItem(part, po.ItemID)
		return &item, err
	}

	if part.ProviderExecuted {
		if ctx.store && po.ItemID != "" {
			item := itemReference(po.ItemID)
			return &item, nil
		}
		if ctx.store || !ctx.hasShellTool || toolName != "shell" {
			return nil, nil
		}
	}

	providerDefined := (ctx.hasLocalShellTool && toolName == "local_shell") ||
		(ctx.hasShellTool && toolName == "shell") ||
		(ctx.hasApplyPatchTool && toolName == "apply_patch") ||
		ctx.isCustomProviderTool(toolName)
	if ctx.hasPreviousResponseID && ctx.store && po.ItemID != "" && providerDefined {
		return nil, nil
	}
	if ctx.store && po.ItemID != "" && providerDefined {
		item := itemReference(po.ItemID)
		return &item, nil
	}

	switch {
	case ctx.hasLocalShellTool && toolName == "local_shell":
		return localShellCallItem(part, po.ItemID)
	case ctx.hasShellTool && toolName == "shell":
		return shellCallItem(part, po.ItemID)
	case ctx.hasApplyPatchTool && toolName == "apply_patch":
		return applyPatchCallItem(part, po.ItemID)
	case ctx.isCustomProviderTool(toolName):
		item := responses.ResponseInputItemParamOfCustomToolCall(part.ToolCallID, customToolInput(part.Input), toolName)
		if po.ItemID != "" {
			item.OfCustomToolCall.ID = param.NewOpt(po.ItemID)
		}
		return &item, nil
	default:
		item := responses.ResponseInputItemParamOfFunctionCall(serializeToolCallArguments(part.Input), part.ToolCallID, toolName)
		if po.Namespace != "" {
			item.OfFunctionCall.Namespace = param.NewOpt(po.Namespace)
		}
		item.OfFunctionCall.Caller = functionToolCallerParam(po.Caller)
		return &item, nil
	}
}

func convertAssistantToolResult(part provider.ContentPart, ctx inputConversionContext) (*responses.ResponseInputItemUnionParam, []provider.Warning, error) {
	if executionDeniedOutput(part.Output) || ctx.hasConversation {
		return nil, nil, nil
	}

	toolName := ctx.toolNameMapping.toProviderToolName(part.ToolName)
	if toolName == "tool_search" {
		itemID := ctx.partOptions(part).ItemID
		if itemID == "" {
			itemID = part.ToolCallID
		}
		if ctx.store {
			item := itemReference(itemID)
			return &item, nil, nil
		}
		if part.Output == nil || part.Output.Type != provider.ToolOutputJSON {
			return nil, nil, nil
		}
		item, err := toolSearchOutputItem(part.Output.JSON, itemID, "server", "")
		return item, nil, err
	}

	if toolName == "programmatic_tool_calling" {
		itemID := ctx.partOptions(part).ItemID
		if itemID == "" {
			itemID = part.ToolCallID
		}
		if ctx.store {
			item := itemReference(itemID)
			return &item, nil, nil
		}
		if part.Output == nil || part.Output.Type != provider.ToolOutputJSON {
			return nil, nil, nil
		}
		item, err := programmaticToolResultItem(part, itemID)
		return item, nil, err
	}

	if ctx.hasShellTool && toolName == "shell" {
		item, err := shellCallOutputItem(part)
		return item, nil, err
	}

	if ctx.store {
		itemID := ctx.partOptions(part).ItemID
		if itemID == "" {
			itemID = part.ToolCallID
		}
		item := itemReference(itemID)
		return &item, nil, nil
	}

	return nil, []provider.Warning{{
		Type:    provider.WarnOther,
		Message: fmt.Sprintf("Results for OpenAI tool %s are not sent to the API when store is false", part.ToolName),
	}}, nil
}

func convertProviderToolResult(part provider.ContentPart, ctx inputConversionContext) (*responses.ResponseInputItemUnionParam, []provider.Warning, error) {
	if part.Output != nil && part.Output.Type == provider.ToolOutputExecutionDenied && ctx.outputOptions(part.Output).ApprovalID != "" {
		return nil, nil, nil
	}

	toolName := ctx.toolNameMapping.toProviderToolName(part.ToolName)

	switch {
	case toolName == "tool_search" && part.Output != nil && part.Output.Type == provider.ToolOutputJSON:
		item, err := toolSearchOutputItem(part.Output.JSON, "", "client", part.ToolCallID)
		return item, nil, err
	case ctx.hasComputerTool && toolName == "computer":
		item, err := computerCallOutputItem(part)
		return &item, nil, err
	case ctx.hasLocalShellTool && toolName == "local_shell" && part.Output != nil && part.Output.Type == provider.ToolOutputJSON:
		item, err := localShellCallOutputItem(part)
		return item, nil, err
	case ctx.hasShellTool && toolName == "shell" && part.Output != nil && part.Output.Type == provider.ToolOutputJSON:
		item, err := shellCallOutputItem(part)
		return item, nil, err
	case ctx.hasApplyPatchTool && toolName == "apply_patch" && part.Output != nil && part.Output.Type == provider.ToolOutputJSON:
		item, err := applyPatchCallOutputItem(part)
		return item, nil, err
	case ctx.isCustomProviderTool(toolName):
		item, warnings := customToolCallOutputItem(part, ctx)
		return item, warnings, nil
	default:
		item := responses.ResponseInputItemParamOfFunctionCallOutput(part.ToolCallID, toolResultOutputString(part.Output, ctx.hasOutputSchema(part.ToolName)))
		item.OfFunctionCallOutput.Caller = functionCallOutputCallerParam(ctx.partOptions(part).Caller)
		return &item, nil, nil
	}
}

func programmaticToolCallItem(part provider.ContentPart, itemID string) (*responses.ResponseInputItemUnionParam, error) {
	var input struct {
		Code        *string `json:"code"`
		Fingerprint *string `json:"fingerprint"`
	}
	if err := json.Unmarshal(part.Input, &input); err != nil {
		return nil, fmt.Errorf("openai: parsing programmatic tool input: %w", err)
	}
	if input.Code == nil || input.Fingerprint == nil {
		return nil, fmt.Errorf("openai: parsing programmatic tool input: code and fingerprint are required")
	}
	if itemID == "" {
		itemID = part.ToolCallID
	}
	item := responses.ResponseInputItemUnionParam{OfProgram: &responses.ResponseInputItemProgramParam{
		ID: itemID, CallID: part.ToolCallID, Code: *input.Code, Fingerprint: *input.Fingerprint,
	}}
	return &item, nil
}

func programmaticToolResultItem(part provider.ContentPart, itemID string) (*responses.ResponseInputItemUnionParam, error) {
	if part.Output == nil || part.Output.Type != provider.ToolOutputJSON {
		return nil, fmt.Errorf("openai: programmatic tool result must use JSON output")
	}
	var output struct {
		Result *string `json:"result"`
		Status *string `json:"status"`
	}
	if err := json.Unmarshal(part.Output.JSON, &output); err != nil {
		return nil, fmt.Errorf("openai: parsing programmatic tool result: %w", err)
	}
	if output.Result == nil || output.Status == nil {
		return nil, fmt.Errorf("openai: parsing programmatic tool result: result and status are required")
	}
	if *output.Status != "completed" && *output.Status != "incomplete" {
		return nil, fmt.Errorf("openai: parsing programmatic tool result: invalid status %q", *output.Status)
	}
	item := responses.ResponseInputItemUnionParam{OfProgramOutput: &responses.ResponseInputItemProgramOutputParam{
		ID: itemID, CallID: part.ToolCallID, Result: *output.Result, Status: *output.Status,
	}}
	return &item, nil
}

func functionToolCallerParam(caller *OpenAIToolCaller) responses.ResponseFunctionToolCallCallerUnionParam {
	if caller == nil {
		return responses.ResponseFunctionToolCallCallerUnionParam{}
	}
	if caller.Type == OpenAIToolCallerProgram {
		return responses.ResponseFunctionToolCallCallerUnionParam{OfProgram: &responses.ResponseFunctionToolCallCallerProgramParam{CallerID: caller.CallerID}}
	}
	direct := responses.NewResponseFunctionToolCallCallerDirectParam()
	return responses.ResponseFunctionToolCallCallerUnionParam{OfDirect: &direct}
}

func functionCallOutputCallerParam(caller *OpenAIToolCaller) responses.ResponseInputItemFunctionCallOutputCallerUnionParam {
	if caller == nil {
		return responses.ResponseInputItemFunctionCallOutputCallerUnionParam{}
	}
	if caller.Type == OpenAIToolCallerProgram {
		return responses.ResponseInputItemFunctionCallOutputCallerUnionParam{OfProgram: &responses.ResponseInputItemFunctionCallOutputCallerProgramParam{CallerID: caller.CallerID}}
	}
	direct := responses.NewResponseInputItemFunctionCallOutputCallerDirectParam()
	return responses.ResponseInputItemFunctionCallOutputCallerUnionParam{OfDirect: &direct}
}

func toolSearchCallItem(part provider.ContentPart, itemID string) (*responses.ResponseInputItemUnionParam, error) {
	var input struct {
		Arguments any     `json:"arguments"`
		CallID    *string `json:"call_id"`
	}
	if err := json.Unmarshal(part.Input, &input); err != nil {
		return nil, fmt.Errorf("openai: parsing tool search input: %w", err)
	}
	item := responses.ResponseInputItemParamOfToolSearchCall(input.Arguments)
	item.OfToolSearchCall.Status = "completed"
	item.OfToolSearchCall.ID = param.NewOpt(itemID)
	if itemID == "" {
		item.OfToolSearchCall.ID = param.NewOpt(part.ToolCallID)
	}
	if input.CallID == nil {
		item.OfToolSearchCall.Execution = "server"
		item.OfToolSearchCall.CallID = param.Null[string]()
	} else {
		item.OfToolSearchCall.Execution = "client"
		item.OfToolSearchCall.CallID = param.NewOpt(*input.CallID)
	}
	return &item, nil
}

func toolSearchOutputItem(raw json.RawMessage, itemID, execution, callID string) (*responses.ResponseInputItemUnionParam, error) {
	var output struct {
		Tools *[]map[string]json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("openai: parsing tool search output: %w", err)
	}
	if output.Tools == nil {
		return nil, fmt.Errorf("openai: parsing tool search output: tools is required")
	}
	for i, tool := range *output.Tools {
		if tool == nil {
			return nil, fmt.Errorf("openai: parsing tool search output: tools[%d] must be an object", i)
		}
	}
	value := map[string]any{
		"type":      "tool_search_output",
		"tools":     *output.Tools,
		"execution": execution,
		"status":    "completed",
	}
	if itemID != "" {
		value["id"] = itemID
	}
	if callID != "" {
		value["call_id"] = callID
	} else {
		value["call_id"] = nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling tool search output: %w", err)
	}
	itemValue := param.Override[responses.ResponseToolSearchOutputItemParam](json.RawMessage(encoded))
	item := responses.ResponseInputItemUnionParam{OfToolSearchOutput: &itemValue}
	return &item, nil
}

func localShellCallItem(part provider.ContentPart, itemID string) (*responses.ResponseInputItemUnionParam, error) {
	var input struct {
		Action map[string]json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal(part.Input, &input); err != nil {
		return nil, fmt.Errorf("openai: parsing local shell input: %w", err)
	}
	if input.Action == nil {
		return nil, fmt.Errorf("openai: parsing local shell input: action is required")
	}
	actionType, err := requiredString(input.Action["type"], "action.type")
	if err != nil || actionType != "exec" {
		return nil, fmt.Errorf("openai: parsing local shell input: action.type must be exec")
	}
	command, err := requiredStringSlice(input.Action["command"], "action.command")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing local shell input: %w", err)
	}
	timeout, err := optionalNumber(input.Action["timeoutMs"], "action.timeoutMs")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing local shell input: %w", err)
	}
	user, err := optionalString(input.Action["user"], "action.user")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing local shell input: %w", err)
	}
	workingDirectory, err := optionalString(input.Action["workingDirectory"], "action.workingDirectory")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing local shell input: %w", err)
	}
	env, err := optionalStringMap(input.Action["env"], "action.env")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing local shell input: %w", err)
	}
	action := map[string]any{"type": "exec", "command": command}
	if timeout != nil {
		action["timeout_ms"] = *timeout
	}
	if user != nil {
		action["user"] = *user
	}
	if workingDirectory != nil {
		action["working_directory"] = *workingDirectory
	}
	if env != nil {
		action["env"] = env
	}
	value := map[string]any{"type": "local_shell_call", "action": action, "call_id": part.ToolCallID}
	if itemID != "" {
		value["id"] = itemID
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling local shell input: %w", err)
	}
	call := param.Override[responses.ResponseInputItemLocalShellCallParam](json.RawMessage(encoded))
	item := responses.ResponseInputItemUnionParam{OfLocalShellCall: &call}
	return &item, nil
}

func localShellCallOutputItem(part provider.ContentPart) (*responses.ResponseInputItemUnionParam, error) {
	var output struct {
		Output *string `json:"output"`
	}
	if err := json.Unmarshal(part.Output.JSON, &output); err != nil {
		return nil, fmt.Errorf("openai: parsing local shell output: %w", err)
	}
	if output.Output == nil {
		return nil, fmt.Errorf("openai: parsing local shell output: output is required")
	}
	raw, err := json.Marshal(map[string]any{
		"type":    "local_shell_call_output",
		"call_id": part.ToolCallID,
		"output":  *output.Output,
	})
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling local shell output: %w", err)
	}
	value := param.Override[responses.ResponseInputItemLocalShellCallOutputParam](json.RawMessage(raw))
	item := responses.ResponseInputItemUnionParam{OfLocalShellCallOutput: &value}
	return &item, nil
}

func shellCallItem(part provider.ContentPart, itemID string) (*responses.ResponseInputItemUnionParam, error) {
	var input struct {
		Action map[string]json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal(part.Input, &input); err != nil {
		return nil, fmt.Errorf("openai: parsing shell input: %w", err)
	}
	if input.Action == nil {
		return nil, fmt.Errorf("openai: parsing shell input: action is required")
	}
	commands, err := requiredStringSlice(input.Action["commands"], "action.commands")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing shell input: %w", err)
	}
	timeout, err := optionalNumber(input.Action["timeoutMs"], "action.timeoutMs")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing shell input: %w", err)
	}
	maxOutputLength, err := optionalNumber(input.Action["maxOutputLength"], "action.maxOutputLength")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing shell input: %w", err)
	}
	action := map[string]any{"commands": commands}
	if timeout != nil {
		action["timeout_ms"] = *timeout
	}
	if maxOutputLength != nil {
		action["max_output_length"] = *maxOutputLength
	}
	value := map[string]any{
		"type":    "shell_call",
		"action":  action,
		"call_id": part.ToolCallID,
		"status":  "completed",
	}
	if itemID != "" {
		value["id"] = itemID
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling shell input: %w", err)
	}
	call := param.Override[responses.ResponseInputItemShellCallParam](json.RawMessage(encoded))
	item := responses.ResponseInputItemUnionParam{OfShellCall: &call}
	return &item, nil
}

func shellCallOutputItem(part provider.ContentPart) (*responses.ResponseInputItemUnionParam, error) {
	if part.Output == nil || part.Output.Type != provider.ToolOutputJSON {
		return nil, nil
	}
	var output struct {
		Output *[]map[string]json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(part.Output.JSON, &output); err != nil {
		return nil, fmt.Errorf("openai: parsing shell output: %w", err)
	}
	if output.Output == nil {
		return nil, fmt.Errorf("openai: parsing shell output: output is required")
	}
	contents := make([]map[string]any, 0, len(*output.Output))
	for i, value := range *output.Output {
		if value == nil {
			return nil, fmt.Errorf("openai: parsing shell output: output[%d] must be an object", i)
		}
		stdout, err := requiredString(value["stdout"], fmt.Sprintf("output[%d].stdout", i))
		if err != nil {
			return nil, fmt.Errorf("openai: parsing shell output: %w", err)
		}
		stderr, err := requiredString(value["stderr"], fmt.Sprintf("output[%d].stderr", i))
		if err != nil {
			return nil, fmt.Errorf("openai: parsing shell output: %w", err)
		}
		var outcome map[string]json.RawMessage
		if err := json.Unmarshal(value["outcome"], &outcome); err != nil || outcome == nil {
			return nil, fmt.Errorf("openai: parsing shell output: output[%d].outcome must be an object", i)
		}
		outcomeType, err := requiredString(outcome["type"], fmt.Sprintf("output[%d].outcome.type", i))
		if err != nil {
			return nil, fmt.Errorf("openai: parsing shell output: %w", err)
		}
		convertedOutcome := map[string]any{"type": outcomeType}
		switch outcomeType {
		case "timeout":
		case "exit":
			exitCode, err := requiredNumber(outcome["exitCode"], fmt.Sprintf("output[%d].outcome.exitCode", i))
			if err != nil {
				return nil, fmt.Errorf("openai: parsing shell output: %w", err)
			}
			convertedOutcome["exit_code"] = exitCode
		default:
			return nil, fmt.Errorf("openai: parsing shell output: output[%d].outcome.type is invalid", i)
		}
		contents = append(contents, map[string]any{"stdout": stdout, "stderr": stderr, "outcome": convertedOutcome})
	}
	encoded, err := json.Marshal(map[string]any{
		"type":    "shell_call_output",
		"call_id": part.ToolCallID,
		"output":  contents,
	})
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling shell output: %w", err)
	}
	value := param.Override[responses.ResponseInputItemShellCallOutputParam](json.RawMessage(encoded))
	item := responses.ResponseInputItemUnionParam{OfShellCallOutput: &value}
	return &item, nil
}

func applyPatchCallItem(part provider.ContentPart, itemID string) (*responses.ResponseInputItemUnionParam, error) {
	var input struct {
		CallID    *string `json:"callId"`
		Operation *struct {
			Type *string `json:"type"`
			Path *string `json:"path"`
			Diff *string `json:"diff"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(part.Input, &input); err != nil {
		return nil, fmt.Errorf("openai: parsing apply patch input: %w", err)
	}
	if input.CallID == nil {
		return nil, fmt.Errorf("openai: parsing apply patch input: callId is required")
	}
	if input.Operation == nil || input.Operation.Type == nil || input.Operation.Path == nil {
		return nil, fmt.Errorf("openai: parsing apply patch input: operation type and path are required")
	}
	callID := *input.CallID
	var item responses.ResponseInputItemUnionParam
	switch *input.Operation.Type {
	case "create_file":
		if input.Operation.Diff == nil {
			return nil, fmt.Errorf("openai: parsing apply patch input: operation.diff is required for create_file")
		}
		item = responses.ResponseInputItemParamOfApplyPatchCall(callID, responses.ResponseInputItemApplyPatchCallOperationCreateFileParam{Path: *input.Operation.Path, Diff: *input.Operation.Diff}, "completed")
	case "delete_file":
		item = responses.ResponseInputItemParamOfApplyPatchCall(callID, responses.ResponseInputItemApplyPatchCallOperationDeleteFileParam{Path: *input.Operation.Path}, "completed")
	case "update_file":
		if input.Operation.Diff == nil {
			return nil, fmt.Errorf("openai: parsing apply patch input: operation.diff is required for update_file")
		}
		item = responses.ResponseInputItemParamOfApplyPatchCall(callID, responses.ResponseInputItemApplyPatchCallOperationUpdateFileParam{Path: *input.Operation.Path, Diff: *input.Operation.Diff}, "completed")
	default:
		return nil, fmt.Errorf("openai: unsupported apply patch operation %q", *input.Operation.Type)
	}
	if itemID != "" {
		item.OfApplyPatchCall.ID = param.NewOpt(itemID)
	}
	return &item, nil
}

func applyPatchCallOutputItem(part provider.ContentPart) (*responses.ResponseInputItemUnionParam, error) {
	var output map[string]json.RawMessage
	if err := json.Unmarshal(part.Output.JSON, &output); err != nil || output == nil {
		return nil, fmt.Errorf("openai: parsing apply patch output: output must be an object")
	}
	status, err := requiredString(output["status"], "status")
	if err != nil || (status != "completed" && status != "failed") {
		return nil, fmt.Errorf("openai: parsing apply patch output: status must be completed or failed")
	}
	text, err := optionalString(output["output"], "output")
	if err != nil {
		return nil, fmt.Errorf("openai: parsing apply patch output: %w", err)
	}
	item := responses.ResponseInputItemParamOfApplyPatchCallOutput(part.ToolCallID, status)
	if text != nil {
		item.OfApplyPatchCallOutput.Output = param.NewOpt(*text)
	}
	return &item, nil
}

func customToolCallOutputItem(part provider.ContentPart, ctx inputConversionContext) (*responses.ResponseInputItemUnionParam, []provider.Warning) {
	if part.Output == nil {
		item := responses.ResponseInputItemParamOfCustomToolCallOutput(part.ToolCallID, "")
		return &item, nil
	}
	if part.Output.Type != provider.ToolOutputContent {
		item := responses.ResponseInputItemParamOfCustomToolCallOutput(part.ToolCallID, toolResultOutputString(part.Output, false))
		return &item, nil
	}

	content := make([]responses.ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam, 0, len(part.Output.Content))
	var warnings []provider.Warning
	for _, value := range part.Output.Content {
		switch value.Type {
		case provider.ToolContentText:
			text := responses.ResponseInputTextParam{Text: value.Text}
			if breakpoint := ctx.contentOptions(value).PromptCacheBreakpoint; breakpoint != nil {
				text.SetExtraFields(map[string]any{"prompt_cache_breakpoint": breakpoint})
			}
			content = append(content, responses.ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam{OfInputText: &text})
		case provider.ToolContentFileURL:
			options := ctx.contentOptions(value)
			if topLevelMediaType(value.MediaType) == "image" {
				image := responses.ResponseInputImageParam{ImageURL: param.NewOpt(value.URL)}
				if options.ImageDetail != "" {
					image.Detail = responses.ResponseInputImageDetail(options.ImageDetail)
				}
				if options.PromptCacheBreakpoint != nil {
					image.SetExtraFields(map[string]any{"prompt_cache_breakpoint": options.PromptCacheBreakpoint})
				}
				content = append(content, responses.ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam{OfInputImage: &image})
			} else {
				file := responses.ResponseInputFileParam{FileURL: param.NewOpt(value.URL)}
				if options.PromptCacheBreakpoint != nil {
					file.SetExtraFields(map[string]any{"prompt_cache_breakpoint": options.PromptCacheBreakpoint})
				}
				content = append(content, responses.ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam{OfInputFile: &file})
			}
		case provider.ToolContentFileData:
			options := ctx.contentOptions(value)
			uri := dataURI(value.MediaType, value.Data)
			if topLevelMediaType(value.MediaType) == "image" {
				image := responses.ResponseInputImageParam{ImageURL: param.NewOpt(uri)}
				if options.ImageDetail != "" {
					image.Detail = responses.ResponseInputImageDetail(options.ImageDetail)
				}
				if options.PromptCacheBreakpoint != nil {
					image.SetExtraFields(map[string]any{"prompt_cache_breakpoint": options.PromptCacheBreakpoint})
				}
				content = append(content, responses.ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam{OfInputImage: &image})
			} else {
				filename := value.Filename
				if filename == "" {
					filename = "data"
				}
				file := responses.ResponseInputFileParam{FileData: param.NewOpt(uri), Filename: param.NewOpt(filename)}
				if options.PromptCacheBreakpoint != nil {
					file.SetExtraFields(map[string]any{"prompt_cache_breakpoint": options.PromptCacheBreakpoint})
				}
				content = append(content, responses.ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam{OfInputFile: &file})
			}
		default:
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnOther,
				Message: fmt.Sprintf("unsupported custom tool content part type: %s", value.Type),
			})
		}
	}
	item := responses.ResponseInputItemParamOfCustomToolCallOutput(part.ToolCallID, content)
	return &item, warnings
}

func requiredString(raw json.RawMessage, name string) (string, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", fmt.Errorf("%s is required", name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return value, nil
}

func optionalString(raw json.RawMessage, name string) (*string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s must be a string", name)
	}
	value, err := requiredString(raw, name)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func requiredStringSlice(raw json.RawMessage, name string) ([]string, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, fmt.Errorf("%s is required", name)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return nil, fmt.Errorf("%s must be an array of strings", name)
	}
	result := make([]string, 0, len(values))
	for _, rawValue := range values {
		value, err := requiredString(rawValue, name)
		if err != nil {
			return nil, fmt.Errorf("%s must be an array of strings", name)
		}
		result = append(result, value)
	}
	return result, nil
}

func requiredNumber(raw json.RawMessage, name string) (json.Number, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", fmt.Errorf("%s is required", name)
	}
	var value json.Number
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a number", name)
	}
	return value, nil
}

func optionalNumber(raw json.RawMessage, name string) (*json.Number, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s must be a number", name)
	}
	value, err := requiredNumber(raw, name)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func optionalStringMap(raw json.RawMessage, name string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s must be an object of strings", name)
	}
	var rawValues map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawValues); err != nil || rawValues == nil {
		return nil, fmt.Errorf("%s must be an object of strings", name)
	}
	values := make(map[string]string, len(rawValues))
	for key, rawValue := range rawValues {
		value, err := requiredString(rawValue, name)
		if err != nil {
			return nil, fmt.Errorf("%s must be an object of strings", name)
		}
		values[key] = value
	}
	return values, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func customToolInput(input json.RawMessage) string {
	var text string
	if json.Unmarshal(input, &text) == nil {
		return text
	}
	return string(input)
}

func executionDeniedOutput(output *provider.ToolResultOutput) bool {
	if output == nil {
		return false
	}
	if output.Type == provider.ToolOutputExecutionDenied {
		return true
	}
	if output.Type != provider.ToolOutputJSON {
		return false
	}
	var wrapped struct {
		Type provider.ToolResultOutputType `json:"type"`
	}
	return json.Unmarshal(output.JSON, &wrapped) == nil && wrapped.Type == provider.ToolOutputExecutionDenied
}

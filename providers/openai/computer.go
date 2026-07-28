package openai

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

type computerActionType string

const (
	computerActionClick       computerActionType = "click"
	computerActionDoubleClick computerActionType = "double_click"
	computerActionDrag        computerActionType = "drag"
	computerActionKeypress    computerActionType = "keypress"
	computerActionMove        computerActionType = "move"
	computerActionScreenshot  computerActionType = "screenshot"
	computerActionScroll      computerActionType = "scroll"
	computerActionTypeText    computerActionType = "type"
	computerActionWait        computerActionType = "wait"
)

type computerCallStatus string

const (
	computerStatusInProgress computerCallStatus = "in_progress"
	computerStatusCompleted  computerCallStatus = "completed"
	computerStatusIncomplete computerCallStatus = "incomplete"
)

type computerButton string

const (
	computerButtonLeft    computerButton = "left"
	computerButtonRight   computerButton = "right"
	computerButtonWheel   computerButton = "wheel"
	computerButtonBack    computerButton = "back"
	computerButtonForward computerButton = "forward"
)

type computerScreenshotType string

const computerScreenshotImage computerScreenshotType = "computer_screenshot"

type computerScreenshotDetail string

const (
	computerDetailAuto     computerScreenshotDetail = "auto"
	computerDetailLow      computerScreenshotDetail = "low"
	computerDetailHigh     computerScreenshotDetail = "high"
	computerDetailOriginal computerScreenshotDetail = "original"
)

type computerItemType string

const (
	computerItemCall       computerItemType = "computer_call"
	computerItemCallOutput computerItemType = "computer_call_output"
)

type computerSafetyCheck struct {
	ID      *string `json:"id,omitempty"`
	Code    *string `json:"code,omitempty"`
	Message *string `json:"message,omitempty"`
}

type computerActionPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type computerAction struct {
	Type    computerActionType     `json:"type"`
	Button  *computerButton        `json:"button,omitempty"`
	X       *float64               `json:"x,omitempty"`
	Y       *float64               `json:"y,omitempty"`
	Keys    *[]string              `json:"keys,omitempty"`
	Path    *[]computerActionPoint `json:"path,omitempty"`
	ScrollX *float64               `json:"scrollX,omitempty"`
	ScrollY *float64               `json:"scrollY,omitempty"`
	Text    *string                `json:"text,omitempty"`
}

type computerWireAction struct {
	Type    computerActionType     `json:"type"`
	Button  *computerButton        `json:"button,omitempty"`
	X       *float64               `json:"x,omitempty"`
	Y       *float64               `json:"y,omitempty"`
	Keys    *[]string              `json:"keys,omitempty"`
	Path    *[]computerActionPoint `json:"path,omitempty"`
	ScrollX *float64               `json:"scroll_x,omitempty"`
	ScrollY *float64               `json:"scroll_y,omitempty"`
	Text    *string                `json:"text,omitempty"`
}

type computerCallInput struct {
	Actions             []computerAction      `json:"actions"`
	PendingSafetyChecks []computerSafetyCheck `json:"pendingSafetyChecks"`
	Status              computerCallStatus    `json:"status"`
}

type computerCallWire struct {
	Type                computerItemType      `json:"type"`
	ID                  string                `json:"id,omitempty"`
	CallID              string                `json:"call_id"`
	Actions             []computerWireAction  `json:"actions"`
	PendingSafetyChecks []computerSafetyCheck `json:"pending_safety_checks"`
	Status              computerCallStatus    `json:"status"`
}

type computerScreenshot struct {
	Type     computerScreenshotType    `json:"type"`
	ImageURL *string                   `json:"imageUrl,omitempty"`
	FileID   *string                   `json:"fileId,omitempty"`
	Detail   *computerScreenshotDetail `json:"detail,omitempty"`
}

type computerCallOutput struct {
	Output                   computerScreenshot     `json:"output"`
	AcknowledgedSafetyChecks *[]computerSafetyCheck `json:"acknowledgedSafetyChecks,omitempty"`
}

type computerScreenshotWire struct {
	Type     computerScreenshotType    `json:"type"`
	ImageURL *string                   `json:"image_url,omitempty"`
	FileID   *string                   `json:"file_id,omitempty"`
	Detail   *computerScreenshotDetail `json:"detail,omitempty"`
}

type computerCallOutputWire struct {
	Type                     computerItemType       `json:"type"`
	CallID                   string                 `json:"call_id"`
	Output                   computerScreenshotWire `json:"output"`
	AcknowledgedSafetyChecks *[]computerSafetyCheck `json:"acknowledged_safety_checks,omitempty"`
}

func mapComputerCallInput(call responses.ResponseComputerToolCall) (json.RawMessage, error) {
	raw := call.RawJSON()
	if raw == "" {
		encoded, err := json.Marshal(call)
		if err != nil {
			return nil, fmt.Errorf("openai: marshaling computer call: %w", err)
		}
		raw = string(encoded)
	}
	var value struct {
		Action              *computerWireAction   `json:"action"`
		Actions             []computerWireAction  `json:"actions"`
		PendingSafetyChecks []computerSafetyCheck `json:"pending_safety_checks"`
		Status              computerCallStatus    `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("openai: decoding computer call: %w", err)
	}
	if value.Actions == nil && value.Action != nil {
		value.Actions = []computerWireAction{*value.Action}
	}
	if value.Actions == nil {
		value.Actions = []computerWireAction{}
	}
	if value.PendingSafetyChecks == nil {
		value.PendingSafetyChecks = []computerSafetyCheck{}
	}
	if value.Status == "" {
		value.Status = computerCallStatus(call.Status)
	}

	mapped := computerCallInput{
		Actions:             make([]computerAction, len(value.Actions)),
		PendingSafetyChecks: value.PendingSafetyChecks,
		Status:              value.Status,
	}
	for i, action := range value.Actions {
		mapped.Actions[i] = mapComputerWireAction(action)
	}
	encoded, err := json.Marshal(mapped)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling computer call input: %w", err)
	}
	return encoded, nil
}

func computerCallInputItem(part provider.ContentPart, itemID string) (responses.ResponseInputItemUnionParam, error) {
	var input computerCallInput
	if err := json.Unmarshal(part.Input, &input); err != nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("openai: decoding computer tool input: %w", err)
	}
	if err := validateComputerCallInputJSON(part.Input); err != nil {
		return responses.ResponseInputItemUnionParam{}, err
	}
	if err := validateComputerCallInput(input); err != nil {
		return responses.ResponseInputItemUnionParam{}, err
	}

	actions := make([]computerWireAction, len(input.Actions))
	for i, action := range input.Actions {
		actions[i] = mapComputerAction(action)
	}
	wire := computerCallWire{
		Type:                computerItemCall,
		ID:                  itemID,
		CallID:              part.ToolCallID,
		Actions:             actions,
		PendingSafetyChecks: input.PendingSafetyChecks,
		Status:              input.Status,
	}
	return rawInputItem(wire)
}

func computerCallOutputItem(part provider.ContentPart) (responses.ResponseInputItemUnionParam, error) {
	if part.Output == nil || part.Output.Type != provider.ToolOutputJSON {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("openai: computer tool output must be JSON")
	}
	var output computerCallOutput
	if err := json.Unmarshal(part.Output.JSON, &output); err != nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("openai: decoding computer tool output: %w", err)
	}
	if err := validateComputerCallOutputJSON(part.Output.JSON); err != nil {
		return responses.ResponseInputItemUnionParam{}, err
	}
	if output.Output.Type != computerScreenshotImage {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("openai: computer tool output type must be computer_screenshot")
	}
	if output.Output.ImageURL == nil && output.Output.FileID == nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("openai: computer screenshot requires imageUrl or fileId")
	}
	if output.Output.Detail != nil {
		switch *output.Output.Detail {
		case computerDetailAuto, computerDetailLow, computerDetailHigh, computerDetailOriginal:
		default:
			return responses.ResponseInputItemUnionParam{}, fmt.Errorf("openai: invalid computer screenshot detail %q", *output.Output.Detail)
		}
	}
	if output.AcknowledgedSafetyChecks != nil {
		if err := validateComputerSafetyChecks(*output.AcknowledgedSafetyChecks); err != nil {
			return responses.ResponseInputItemUnionParam{}, err
		}
	}
	wire := computerCallOutputWire{
		Type:   computerItemCallOutput,
		CallID: part.ToolCallID,
		Output: computerScreenshotWire{
			Type:     output.Output.Type,
			ImageURL: output.Output.ImageURL,
			FileID:   output.Output.FileID,
			Detail:   output.Output.Detail,
		},
		AcknowledgedSafetyChecks: output.AcknowledgedSafetyChecks,
	}
	return rawInputItem(wire)
}

func mapComputerWireAction(action computerWireAction) computerAction {
	mapped := computerAction{Type: action.Type}
	switch action.Type {
	case computerActionClick:
		mapped.Button, mapped.X, mapped.Y, mapped.Keys = action.Button, action.X, action.Y, action.Keys
	case computerActionDoubleClick, computerActionMove:
		mapped.X, mapped.Y, mapped.Keys = action.X, action.Y, action.Keys
	case computerActionDrag:
		mapped.Path, mapped.Keys = action.Path, action.Keys
	case computerActionKeypress:
		mapped.Keys = action.Keys
	case computerActionScroll:
		mapped.X, mapped.Y, mapped.ScrollX, mapped.ScrollY, mapped.Keys = action.X, action.Y, action.ScrollX, action.ScrollY, action.Keys
	case computerActionTypeText:
		mapped.Text = action.Text
	}
	return mapped
}

func mapComputerAction(action computerAction) computerWireAction {
	mapped := computerWireAction{Type: action.Type}
	switch action.Type {
	case computerActionClick:
		mapped.Button, mapped.X, mapped.Y, mapped.Keys = action.Button, action.X, action.Y, action.Keys
	case computerActionDoubleClick, computerActionMove:
		mapped.X, mapped.Y, mapped.Keys = action.X, action.Y, action.Keys
	case computerActionDrag:
		mapped.Path, mapped.Keys = action.Path, action.Keys
	case computerActionKeypress:
		mapped.Keys = action.Keys
	case computerActionScroll:
		mapped.X, mapped.Y, mapped.ScrollX, mapped.ScrollY, mapped.Keys = action.X, action.Y, action.ScrollX, action.ScrollY, action.Keys
	case computerActionTypeText:
		mapped.Text = action.Text
	}
	return mapped
}

func rawInputItem(value any) (responses.ResponseInputItemUnionParam, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("openai: marshaling response input item: %w", err)
	}
	return param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(encoded)), nil
}

func validateComputerCallInputJSON(data json.RawMessage) error {
	var raw struct {
		Actions             []json.RawMessage `json:"actions"`
		PendingSafetyChecks []json.RawMessage `json:"pendingSafetyChecks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("openai: decoding computer tool input fields: %w", err)
	}
	for _, action := range raw.Actions {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(action, &fields); err != nil {
			continue
		}
		var actionType computerActionType
		if err := json.Unmarshal(fields["type"], &actionType); err != nil {
			continue
		}
		switch actionType {
		case computerActionClick, computerActionDoubleClick, computerActionDrag, computerActionMove, computerActionScroll:
			if value, ok := fields["keys"]; ok && isJSONNull(value) {
				return fmt.Errorf("openai: computer action keys must not be null")
			}
		}
	}
	return validateComputerSafetyCheckJSON(raw.PendingSafetyChecks)
}

func validateComputerCallOutputJSON(data json.RawMessage) error {
	var raw struct {
		Output                   map[string]json.RawMessage `json:"output"`
		AcknowledgedSafetyChecks json.RawMessage            `json:"acknowledgedSafetyChecks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("openai: decoding computer tool output fields: %w", err)
	}
	for _, field := range []string{"imageUrl", "fileId", "detail"} {
		if value, ok := raw.Output[field]; ok && isJSONNull(value) {
			return fmt.Errorf("openai: computer screenshot %s must not be null", field)
		}
	}
	if raw.AcknowledgedSafetyChecks == nil {
		return nil
	}
	if isJSONNull(raw.AcknowledgedSafetyChecks) {
		return fmt.Errorf("openai: acknowledgedSafetyChecks must not be null")
	}
	var checks []json.RawMessage
	if err := json.Unmarshal(raw.AcknowledgedSafetyChecks, &checks); err != nil {
		return fmt.Errorf("openai: decoding acknowledged safety checks: %w", err)
	}
	return validateComputerSafetyCheckJSON(checks)
}

func validateComputerSafetyCheckJSON(checks []json.RawMessage) error {
	for _, check := range checks {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(check, &fields); err != nil {
			continue
		}
		for _, field := range []string{"code", "message"} {
			if value, ok := fields[field]; ok && isJSONNull(value) {
				return fmt.Errorf("openai: computer safety check %s must not be null", field)
			}
		}
	}
	return nil
}

func validateComputerCallInput(input computerCallInput) error {
	if input.Actions == nil {
		return fmt.Errorf("openai: computer call actions are required")
	}
	if input.PendingSafetyChecks == nil {
		return fmt.Errorf("openai: computer call pendingSafetyChecks are required")
	}
	if err := validateComputerSafetyChecks(input.PendingSafetyChecks); err != nil {
		return err
	}
	switch input.Status {
	case computerStatusInProgress, computerStatusCompleted, computerStatusIncomplete:
	default:
		return fmt.Errorf("openai: invalid computer call status %q", input.Status)
	}
	for _, action := range input.Actions {
		switch action.Type {
		case computerActionClick:
			if action.Button == nil || !validComputerButton(*action.Button) || action.X == nil || action.Y == nil {
				return fmt.Errorf("openai: computer click action requires a valid button, x, and y")
			}
		case computerActionDoubleClick, computerActionMove:
			if action.X == nil || action.Y == nil {
				return fmt.Errorf("openai: computer action %q requires x and y", action.Type)
			}
		case computerActionDrag:
			if action.Path == nil {
				return fmt.Errorf("openai: computer drag action requires path")
			}
		case computerActionKeypress:
			if action.Keys == nil {
				return fmt.Errorf("openai: computer keypress action requires keys")
			}
		case computerActionScreenshot, computerActionWait:
		case computerActionScroll:
			if action.X == nil || action.Y == nil || action.ScrollX == nil || action.ScrollY == nil {
				return fmt.Errorf("openai: computer scroll action requires x, y, scrollX, and scrollY")
			}
		case computerActionTypeText:
			if action.Text == nil {
				return fmt.Errorf("openai: computer type action requires text")
			}
		default:
			return fmt.Errorf("openai: unsupported computer action %q", action.Type)
		}
	}
	return nil
}

func validateComputerSafetyChecks(checks []computerSafetyCheck) error {
	for _, check := range checks {
		if check.ID == nil {
			return fmt.Errorf("openai: computer safety check id is required")
		}
	}
	return nil
}

func validComputerButton(button computerButton) bool {
	switch button {
	case computerButtonLeft, computerButtonRight, computerButtonWheel, computerButtonBack, computerButtonForward:
		return true
	default:
		return false
	}
}

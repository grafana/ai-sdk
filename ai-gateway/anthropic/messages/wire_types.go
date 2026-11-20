package messages

type contentBlockType string

const (
	contentBlockText             contentBlockType = "text"
	contentBlockThinking         contentBlockType = "thinking"
	contentBlockRedactedThinking contentBlockType = "redacted_thinking"
	contentBlockToolUse          contentBlockType = "tool_use"
	contentBlockToolResult       contentBlockType = "tool_result"
)

type toolType string

const (
	toolTypeDefault toolType = ""
	toolTypeCustom  toolType = "custom"
)

type toolChoiceType string

const (
	toolChoiceAuto toolChoiceType = "auto"
	toolChoiceAny  toolChoiceType = "any"
	toolChoiceTool toolChoiceType = "tool"
	toolChoiceNone toolChoiceType = "none"
)

type envelopeType string

const (
	envelopeMessage envelopeType = "message"
	envelopeError   envelopeType = "error"
)

type apiErrorType string

const (
	apiErrorInvalidRequest  apiErrorType = "invalid_request_error"
	apiErrorAuthentication  apiErrorType = "authentication_error"
	apiErrorPermission      apiErrorType = "permission_error"
	apiErrorNotFound        apiErrorType = "not_found_error"
	apiErrorRequestTooLarge apiErrorType = "request_too_large"
	apiErrorAPI             apiErrorType = "api_error"
	apiErrorRateLimit       apiErrorType = "rate_limit_error"
	apiErrorOverloaded      apiErrorType = "overloaded_error"
	apiErrorTimeout         apiErrorType = "timeout_error"
)

type eventType string

const (
	eventMessageStart      eventType = "message_start"
	eventContentBlockStart eventType = "content_block_start"
	eventContentBlockDelta eventType = "content_block_delta"
	eventContentBlockStop  eventType = "content_block_stop"
	eventMessageDelta      eventType = "message_delta"
	eventMessageStop       eventType = "message_stop"
	eventError             eventType = "error"
)

type deltaType string

const (
	deltaText      deltaType = "text_delta"
	deltaInputJSON deltaType = "input_json_delta"
)

type stopReasonType string

const (
	stopReasonEndTurn      stopReasonType = "end_turn"
	stopReasonRefusal      stopReasonType = "refusal"
	stopReasonToolUse      stopReasonType = "tool_use"
	stopReasonMaxTokens    stopReasonType = "max_tokens"
	stopReasonPauseTurn    stopReasonType = "pause_turn"
	stopReasonStopSequence stopReasonType = "stop_sequence"
)

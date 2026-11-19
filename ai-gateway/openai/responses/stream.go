package responses

import (
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type eventType string

const (
	eventCreated            eventType = "response.created"
	eventInProgress         eventType = "response.in_progress"
	eventCompleted          eventType = "response.completed"
	eventIncomplete         eventType = "response.incomplete"
	eventFailed             eventType = "response.failed"
	eventItemAdded          eventType = "response.output_item.added"
	eventItemDone           eventType = "response.output_item.done"
	eventContentPartAdded   eventType = "response.content_part.added"
	eventContentPartDone    eventType = "response.content_part.done"
	eventTextDelta          eventType = "response.output_text.delta"
	eventTextDone           eventType = "response.output_text.done"
	eventArgumentsDelta     eventType = "response.function_call_arguments.delta"
	eventArgumentsDone      eventType = "response.function_call_arguments.done"
	eventSummaryPartAdded   eventType = "response.reasoning_summary_part.added"
	eventSummaryPartDone    eventType = "response.reasoning_summary_part.done"
	eventSummaryTextDelta   eventType = "response.reasoning_summary_text.delta"
	eventSummaryTextDone    eventType = "response.reasoning_summary_text.done"
	reasoningBlockClosed    int       = -2
	reasoningBlockNoSummary int       = -1
)

type event struct {
	Type           eventType `json:"type"`
	SequenceNumber int       `json:"sequence_number"`
	Response       *response `json:"response,omitempty"`
	OutputIndex    *int      `json:"output_index,omitempty"`
	ItemID         string    `json:"item_id,omitempty"`
	ContentIndex   *int      `json:"content_index,omitempty"`
	SummaryIndex   *int      `json:"summary_index,omitempty"`
	Item           any       `json:"item,omitempty"`
	Part           any       `json:"part,omitempty"`
	Delta          *string   `json:"delta,omitempty"`
	Text           *string   `json:"text,omitempty"`
	Name           string    `json:"name,omitempty"`
	Arguments      *string   `json:"arguments,omitempty"`
}

type textState struct {
	index int
	item  *messageOutput
	open  bool
}

type toolState struct {
	index        int
	item         *functionCallOutput
	arguments    strings.Builder
	ended, final bool
}

type reasoningState struct {
	index int
	item  *reasoningOutput
	// blocks maps a provider reasoning block to its summary index, or to
	// reasoningBlockNoSummary until its first non-empty delta.
	blocks map[string]int
}

// encoder turns provider stream parts into Responses events. Every event it
// returns is ready to send in order.
type encoder struct {
	r         mappedRequest
	resp      *response
	sequence  int
	texts     map[string]*textState
	tools     map[string]*toolState
	calls     []*functionCallOutput
	reasoning *reasoningState
	text      strings.Builder
	events    []event
}

func newEncoder(r mappedRequest, resp *response) *encoder {
	return &encoder{r: r, resp: resp, texts: map[string]*textState{}, tools: map[string]*toolState{}}
}

func (e *encoder) add(ev event) {
	ev.SequenceNumber = e.sequence
	e.sequence++
	e.events = append(e.events, ev)
}

// take returns and clears the pending events.
func (e *encoder) take() []event {
	events := e.events
	e.events = nil
	return events
}

func (e *encoder) start() {
	e.add(event{Type: eventCreated, Response: snapshot(e.resp)})
	e.add(event{Type: eventInProgress, Response: snapshot(e.resp)})
}

func snapshot(r *response) *response {
	copied := *r
	copied.Output = append([]any{}, r.Output...)
	return &copied
}

func (e *encoder) openItem(prefix string, build func(id string) any) int {
	index := len(e.resp.Output)
	item := build(itemID(prefix, e.resp.ID, index))
	e.resp.Output = append(e.resp.Output, item)
	e.add(event{Type: eventItemAdded, OutputIndex: ptr(index), Item: clone(item)})
	return index
}

func clone(item any) any {
	switch v := item.(type) {
	case *messageOutput:
		c := *v
		c.Content = append([]outputText{}, v.Content...)
		return &c
	case *functionCallOutput:
		c := *v
		return &c
	case *reasoningOutput:
		c := *v
		c.Summary = append([]summaryPart{}, v.Summary...)
		return &c
	}
	return item
}

func (e *encoder) closeReasoning() error {
	s := e.reasoning
	if s == nil {
		return nil
	}
	for _, summary := range s.blocks {
		if summary != reasoningBlockClosed {
			return errOutput
		}
	}
	e.add(event{Type: eventItemDone, OutputIndex: ptr(s.index), Item: clone(s.item)})
	e.reasoning = nil
	return nil
}

func (e *encoder) consume(p provider.StreamPart) error {
	if !utf8.ValidString(p.Delta) || !utf8.ValidString(p.Input) || !utf8.ValidString(p.ID) {
		return errOutput
	}
	switch p.Type {
	case provider.PartStreamStart, provider.PartResponseMeta:
		return nil
	case provider.PartCustom:
		if p.Kind == "anthropic.message_start" {
			return nil
		}
		return errOutput
	case provider.PartReasoningStart:
		if p.ID == "" {
			return errOutput
		}
		if e.reasoning == nil {
			s := &reasoningState{blocks: map[string]int{}}
			s.index = e.openItem("rs", func(id string) any {
				s.item = &reasoningOutput{ID: id, Type: itemReasoning, Summary: []summaryPart{}}
				return s.item
			})
			e.reasoning = s
		}
		if _, ok := e.reasoning.blocks[p.ID]; ok {
			return errOutput
		}
		e.reasoning.blocks[p.ID] = reasoningBlockNoSummary
		return nil
	case provider.PartReasoningDelta:
		s := e.reasoning
		if s == nil {
			return errOutput
		}
		summary, ok := s.blocks[p.ID]
		if !ok || summary == reasoningBlockClosed {
			return errOutput
		}
		if p.Delta == "" {
			return nil
		}
		if summary == reasoningBlockNoSummary {
			summary = len(s.item.Summary)
			s.blocks[p.ID] = summary
			s.item.Summary = append(s.item.Summary, summaryPart{Type: partSummaryText})
			e.add(event{Type: eventSummaryPartAdded, OutputIndex: ptr(s.index), ItemID: s.item.ID, SummaryIndex: ptr(summary), Part: summaryPart{Type: partSummaryText}})
		}
		s.item.Summary[summary].Text += p.Delta
		e.add(event{Type: eventSummaryTextDelta, OutputIndex: ptr(s.index), ItemID: s.item.ID, SummaryIndex: ptr(summary), Delta: ptr(p.Delta)})
		return nil
	case provider.PartReasoningEnd:
		s := e.reasoning
		if s == nil {
			return errOutput
		}
		summary, ok := s.blocks[p.ID]
		if !ok || summary == reasoningBlockClosed {
			return errOutput
		}
		s.blocks[p.ID] = reasoningBlockClosed
		if summary >= 0 {
			part := s.item.Summary[summary]
			e.add(event{Type: eventSummaryTextDone, OutputIndex: ptr(s.index), ItemID: s.item.ID, SummaryIndex: ptr(summary), Text: ptr(part.Text)})
			e.add(event{Type: eventSummaryPartDone, OutputIndex: ptr(s.index), ItemID: s.item.ID, SummaryIndex: ptr(summary), Part: part})
		}
		return nil
	case provider.PartTextStart:
		if p.ID == "" || e.texts[p.ID] != nil {
			return errOutput
		}
		if err := e.closeReasoning(); err != nil {
			return err
		}
		t := &textState{open: true}
		t.index = e.openItem("msg", func(id string) any {
			t.item = &messageOutput{ID: id, Type: itemMessage, Status: statusInProgress, Role: "assistant", Content: []outputText{}}
			return t.item
		})
		t.item.Content = append(t.item.Content, outputText{Type: partOutputText, Annotations: []any{}})
		e.texts[p.ID] = t
		e.add(event{Type: eventContentPartAdded, OutputIndex: ptr(t.index), ItemID: t.item.ID, ContentIndex: ptr(0), Part: outputText{Type: partOutputText, Annotations: []any{}}})
		return nil
	case provider.PartTextDelta:
		t := e.texts[p.ID]
		if t == nil || !t.open {
			return errOutput
		}
		e.text.WriteString(p.Delta)
		if p.Delta == "" {
			return nil
		}
		t.item.Content[0].Text += p.Delta
		e.add(event{Type: eventTextDelta, OutputIndex: ptr(t.index), ItemID: t.item.ID, ContentIndex: ptr(0), Delta: ptr(p.Delta)})
		return nil
	case provider.PartTextEnd:
		t := e.texts[p.ID]
		if t == nil || !t.open {
			return errOutput
		}
		t.open = false
		t.item.Status = statusCompleted
		part := t.item.Content[0]
		e.add(event{Type: eventTextDone, OutputIndex: ptr(t.index), ItemID: t.item.ID, ContentIndex: ptr(0), Text: ptr(part.Text)})
		e.add(event{Type: eventContentPartDone, OutputIndex: ptr(t.index), ItemID: t.item.ID, ContentIndex: ptr(0), Part: part})
		e.add(event{Type: eventItemDone, OutputIndex: ptr(t.index), Item: clone(t.item)})
		return nil
	case provider.PartToolInputStart:
		if p.ID == "" || len(p.ID) > 256 || !namePattern.MatchString(p.ToolName) || e.tools[p.ID] != nil || p.ProviderExecuted || boolValue(p.Dynamic) || (p.InputEncoding != "" && p.InputEncoding != provider.ToolInputEncodingJSON) {
			return errOutput
		}
		if err := e.closeReasoning(); err != nil {
			return err
		}
		e.openTool(p.ID, p.ToolName)
		return nil
	case provider.PartToolInputDelta:
		t := e.tools[p.ID]
		if t == nil || t.ended || t.final {
			return errOutput
		}
		t.arguments.WriteString(p.Delta)
		if p.Delta != "" {
			e.add(event{Type: eventArgumentsDelta, OutputIndex: ptr(t.index), ItemID: t.item.ID, Delta: ptr(p.Delta)})
		}
		return nil
	case provider.PartToolInputEnd:
		t := e.tools[p.ID]
		if t == nil || t.ended || t.final {
			return errOutput
		}
		t.ended = true
		return nil
	case provider.PartToolCall:
		if p.ProviderExecuted || boolValue(p.Dynamic) || boolValue(p.Preliminary) || validateCall(p.ToolCallID, p.ToolName, p.Input, e.r) != nil {
			return errOutput
		}
		t := e.tools[p.ToolCallID]
		if t == nil {
			if err := e.closeReasoning(); err != nil {
				return err
			}
			t = e.openTool(p.ToolCallID, p.ToolName)
			t.ended = true
			t.arguments.WriteString(p.Input)
			e.add(event{Type: eventArgumentsDelta, OutputIndex: ptr(t.index), ItemID: t.item.ID, Delta: ptr(p.Input)})
		}
		if t.final || !t.ended || t.item.Name != p.ToolName {
			return errOutput
		}
		if t.arguments.Len() == 0 && p.Input == "{}" {
			t.arguments.WriteString("{}")
			e.add(event{Type: eventArgumentsDelta, OutputIndex: ptr(t.index), ItemID: t.item.ID, Delta: ptr("{}")})
		}
		if t.arguments.String() != p.Input {
			return errOutput
		}
		t.final = true
		t.item.Arguments = p.Input
		t.item.Status = statusCompleted
		e.calls = append(e.calls, t.item)
		e.add(event{Type: eventArgumentsDone, OutputIndex: ptr(t.index), ItemID: t.item.ID, Name: t.item.Name, Arguments: ptr(p.Input)})
		e.add(event{Type: eventItemDone, OutputIndex: ptr(t.index), Item: clone(t.item)})
		return nil
	default:
		return errOutput
	}
}

func (e *encoder) openTool(id, name string) *toolState {
	t := &toolState{}
	t.index = e.openItem("fc", func(itemID string) any {
		t.item = &functionCallOutput{ID: itemID, Type: itemFunctionCall, Status: statusInProgress, CallID: id, Name: name}
		return t.item
	})
	e.tools[id] = t
	return t
}

// finish validates the complete output and adds the terminal event.
func (e *encoder) finish(p provider.StreamPart) error {
	if p.FinishReason == nil {
		return errOutput
	}
	terminal, reason, err := finish(*p.FinishReason)
	if err != nil {
		return err
	}
	if err := e.closeReasoning(); err != nil {
		return err
	}
	for _, t := range e.texts {
		if t.open {
			return errOutput
		}
	}
	calls := make([]functionCallOutput, 0, len(e.calls))
	for _, t := range e.tools {
		if !t.final {
			return errOutput
		}
	}
	for _, c := range e.calls {
		calls = append(calls, *c)
	}
	if err := validateFinish(e.r, *p.FinishReason, calls, e.text.String()); err != nil {
		return err
	}
	if p.Usage != nil {
		if e.resp.Usage, err = mapUsage(*p.Usage); err != nil {
			return err
		}
	}
	e.resp.Status = terminal
	typ := eventCompleted
	if reason != "" {
		typ = eventIncomplete
		e.resp.IncompleteDetails = &incompleteDetails{reason}
	}
	e.add(event{Type: typ, Response: snapshot(e.resp)})
	return nil
}

// fail adds a response.failed event with the fixed error for status.
func (e *encoder) fail(status int) {
	apiErr := errorFor(status)
	e.resp.Status = statusFailed
	e.resp.Error = &responseError{Code: apiErr.Code, Message: apiErr.Message}
	e.add(event{Type: eventFailed, Response: snapshot(e.resp)})
}

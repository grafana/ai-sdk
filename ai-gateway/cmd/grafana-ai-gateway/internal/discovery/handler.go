package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

var errResponseLimit = errors.New("gateway discovery: response exceeds byte limit")

type model struct {
	ID            string
	Name          string
	Description   string
	Specification specification
}

type specification struct {
	SpecificationVersion string `json:"specificationVersion"`
	Provider             string `json:"provider"`
	ModelID              string `json:"modelId"`
}

type handler struct {
	lister catalog.ModelLister
	errors *providerv4.HostErrorWriter
	limit  int64
}

// New constructs a closed bounded discovery handler.
func New(lister catalog.ModelLister, errorWriter *providerv4.HostErrorWriter, limit int64) (http.Handler, error) {
	if isNil(lister) {
		return nil, fmt.Errorf("gateway discovery: model lister is nil")
	}
	if errorWriter == nil {
		return nil, fmt.Errorf("gateway discovery: error writer is nil")
	}
	if limit <= 0 || limit == int64(^uint64(0)>>1) {
		return nil, fmt.Errorf("gateway discovery: response limit is unsafe")
	}
	return &handler{lister: lister, errors: errorWriter, limit: limit}, nil
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	models, err := handler.safeList(request.Context())
	if err != nil {
		handler.errors.Write(w, providerv4.HostErrorInternal)
		return
	}
	document, err := handler.encode(models)
	if err != nil {
		handler.errors.Write(w, providerv4.HostErrorInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func (handler *handler) safeList(ctx context.Context) (models []catalog.ModelInfo, err error) {
	defer func() {
		if recover() != nil {
			models = nil
			err = fmt.Errorf("gateway discovery: listing models panicked")
		}
	}()
	return handler.lister.ListModels(ctx)
}

type modelRow struct {
	id        string
	infoIndex int
}

func (handler *handler) encode(infos []catalog.ModelInfo) ([]byte, error) {
	rows, err := prepareRows(infos, handler.limit)
	if err != nil {
		return nil, err
	}
	buffer := newBoundedBuffer(handler.limit + 1)
	buffer.append(`{"models":[`)
	for index, row := range rows {
		if index > 0 {
			buffer.append(",")
		}
		info := infos[row.infoIndex]
		encodeModel(&buffer, discoveryModel(row.id, info.Name, info.Description))
		if buffer.overflow || buffer.invalid {
			return nil, errResponseLimit
		}
	}
	buffer.append(`]}`)
	if buffer.overflow || buffer.invalid || int64(len(buffer.data)) > handler.limit {
		return nil, errResponseLimit
	}
	return buffer.data, nil
}

func prepareRows(infos []catalog.ModelInfo, limit int64) ([]modelRow, error) {
	minimumSize := int64(len(`{"models":[]}`))
	rowCount := 0
	for _, info := range infos {
		if !validPublicID(info.ID) || !validString(info.Name) || !utf8.ValidString(info.Description) {
			return nil, fmt.Errorf("gateway discovery: invalid model text")
		}
		if !addMinimumRow(&minimumSize, &rowCount, info.ID, info.Name, limit) {
			return nil, errResponseLimit
		}
		for _, alias := range info.Aliases {
			if !validPublicID(alias) {
				return nil, fmt.Errorf("gateway discovery: invalid alias")
			}
			if !addMinimumRow(&minimumSize, &rowCount, alias, info.Name, limit) {
				return nil, errResponseLimit
			}
		}
	}
	rows := make([]modelRow, 0, rowCount)
	for infoIndex, info := range infos {
		rows = append(rows, modelRow{id: info.ID, infoIndex: infoIndex})
		for _, alias := range info.Aliases {
			rows = append(rows, modelRow{id: alias, infoIndex: infoIndex})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	return rows, nil
}

func addMinimumRow(total *int64, count *int, id, name string, limit int64) bool {
	rowSize := minimumModelSize(id, name)
	if *count > 0 {
		rowSize++
	}
	if rowSize > limit-*total {
		return false
	}
	*total += rowSize
	*count++
	return true
}

func minimumModelSize(id, name string) int64 {
	return int64(len(`{"id":`)+len(`,"name":`)+len(`,"specification":{"specificationVersion":"v4","provider":"grafana","modelId":`)+len(`}}`)) +
		jsonStringSize(id)*2 + jsonStringSize(name)
}

func jsonStringSize(value string) int64 {
	size := int64(2)
	for index := 0; index < len(value); {
		character := value[index]
		switch character {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			size += 2
			index++
		default:
			if character < 0x20 {
				size += 6
				index++
				continue
			}
			_, width := utf8.DecodeRuneInString(value[index:])
			size += int64(width)
			index += width
		}
	}
	return size
}

func discoveryModel(id, name, description string) model {
	return model{
		ID:          id,
		Name:        name,
		Description: description,
		Specification: specification{
			SpecificationVersion: "v4",
			Provider:             "grafana",
			ModelID:              id,
		},
	}
}

func validString(value string) bool {
	return value != "" && utf8.ValidString(value)
}

func validPublicID(value string) bool {
	if len(value) < 1 || len(value) > 128 || !isASCIIAlphanumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isASCIIAlphanumeric(character) && character != '.' && character != '_' && character != ':' && character != '/' && character != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlphanumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9')
}

func encodeModel(buffer *boundedBuffer, value model) {
	buffer.append(`{"id":`)
	buffer.appendJSONString(value.ID)
	buffer.append(`,"name":`)
	buffer.appendJSONString(value.Name)
	if value.Description != "" {
		buffer.append(`,"description":`)
		buffer.appendJSONString(value.Description)
	}
	buffer.append(`,"specification":{"specificationVersion":`)
	buffer.appendJSONString(value.Specification.SpecificationVersion)
	buffer.append(`,"provider":`)
	buffer.appendJSONString(value.Specification.Provider)
	buffer.append(`,"modelId":`)
	buffer.appendJSONString(value.Specification.ModelID)
	buffer.append(`}}`)
}

type boundedBuffer struct {
	data     []byte
	limit    int64
	overflow bool
	invalid  bool
}

func newBoundedBuffer(limit int64) boundedBuffer {
	capacity := 256
	if limit < int64(capacity) {
		capacity = int(limit)
	}
	return boundedBuffer{data: make([]byte, 0, capacity), limit: limit}
}

func (buffer *boundedBuffer) append(value string) {
	if buffer.overflow || buffer.invalid {
		return
	}
	remaining := buffer.limit - int64(len(buffer.data))
	if remaining < 0 {
		buffer.overflow = true
		return
	}
	if int64(len(value)) <= remaining {
		buffer.data = append(buffer.data, value...)
		return
	}
	buffer.data = append(buffer.data, value[:remaining]...)
	buffer.overflow = true
}

func (buffer *boundedBuffer) appendBytes(value []byte) {
	if buffer.overflow || buffer.invalid {
		return
	}
	remaining := buffer.limit - int64(len(buffer.data))
	if remaining < 0 {
		buffer.overflow = true
		return
	}
	if int64(len(value)) <= remaining {
		buffer.data = append(buffer.data, value...)
		return
	}
	buffer.data = append(buffer.data, value[:remaining]...)
	buffer.overflow = true
}

func (buffer *boundedBuffer) appendJSONString(value string) {
	if buffer.overflow || buffer.invalid {
		return
	}
	if !utf8.ValidString(value) {
		buffer.invalid = true
		return
	}
	buffer.append(`"`)
	const hex = "0123456789abcdef"
	for index := 0; index < len(value) && !buffer.overflow; {
		character := value[index]
		switch character {
		case '"', '\\':
			buffer.appendBytes([]byte{'\\', character})
			index++
		case '\b':
			buffer.append(`\b`)
			index++
		case '\f':
			buffer.append(`\f`)
			index++
		case '\n':
			buffer.append(`\n`)
			index++
		case '\r':
			buffer.append(`\r`)
			index++
		case '\t':
			buffer.append(`\t`)
			index++
		default:
			if character < 0x20 {
				buffer.appendBytes([]byte{'\\', 'u', '0', '0', hex[character>>4], hex[character&0x0f]})
				index++
				continue
			}
			_, size := utf8.DecodeRuneInString(value[index:])
			buffer.append(value[index : index+size])
			index += size
		}
	}
	buffer.append(`"`)
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

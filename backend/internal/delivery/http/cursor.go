package httpdelivery

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
)

// maxCursorLength bounds an opaque cursor; a container name and a UUID fit
// well below it.
const maxCursorLength = 1024

var errInvalidCursor = errors.New("cursor is invalid")

// cursorPayload is the content of an opaque cursor (API_CONTRACTS.md
// § 9.1): the sort key of the last item of the page and its id.
type cursorPayload struct {
	Key string `json:"k"`
	ID  string `json:"id"`
}

func encodeCursor(key string, id uuid.UUID) string {
	data, err := json.Marshal(cursorPayload{Key: key, ID: id.String()})
	if err != nil {
		// A string and a UUID always encode.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeCursor(value string) (string, uuid.UUID, error) {
	if len(value) > maxCursorLength {
		return "", uuid.UUID{}, errInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", uuid.UUID{}, errInvalidCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil || decoder.More() {
		return "", uuid.UUID{}, errInvalidCursor
	}
	id, err := uuid.Parse(payload.ID)
	if err != nil || payload.ID != id.String() || payload.Key == "" {
		return "", uuid.UUID{}, errInvalidCursor
	}
	return payload.Key, id, nil
}

func encodeContainerCursor(cursor *domain.ContainerCursor) *string {
	if cursor == nil {
		return nil
	}
	value := encodeCursor(cursor.Name, cursor.ID)
	return &value
}

func decodeContainerCursor(value string) (*domain.ContainerCursor, error) {
	if value == "" {
		return nil, nil
	}
	name, id, err := decodeCursor(value)
	if err != nil {
		return nil, err
	}
	return &domain.ContainerCursor{Name: name, ID: id}, nil
}

func encodeEventCursor(cursor *domain.EventCursor) *string {
	if cursor == nil {
		return nil
	}
	value := encodeCursor(cursor.OccurredAt.UTC().Format(time.RFC3339Nano), cursor.ID)
	return &value
}

func decodeEventCursor(value string) (*domain.EventCursor, error) {
	if value == "" {
		return nil, nil
	}
	key, id, err := decodeCursor(value)
	if err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339Nano, key)
	if err != nil {
		return nil, errInvalidCursor
	}
	return &domain.EventCursor{OccurredAt: at.UTC(), ID: id}, nil
}

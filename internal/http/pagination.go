package httpapi

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const cursorSep = "|"

func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s%s%s", t.UTC().Format(time.RFC3339Nano), cursorSep, id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(raw string) (time.Time, uuid.UUID, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("invalid cursor encoding")
	}
	parts := strings.Split(string(decoded), cursorSep)
	if len(parts) != 2 {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("invalid cursor")
	}
	parsedTime, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("invalid cursor time")
	}
	parsedID, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.UUID{}, fmt.Errorf("invalid cursor id")
	}
	return parsedTime, parsedID, nil
}

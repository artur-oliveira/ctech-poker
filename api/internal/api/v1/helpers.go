package v1

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
)

// PaginatedResponse is the standard envelope for every list endpoint.
type PaginatedResponse struct {
	Data           any     `json:"data"`
	HasNext        bool    `json:"has_next"`
	NextCursor     *string `json:"next_cursor"`
	HasPrevious    bool    `json:"has_previous"`
	PreviousCursor *string `json:"previous_cursor"`
}

// cursorPayload is the JSON structure embedded in every base64 cursor: the
// DynamoDB ExclusiveStartKey, serialized as a plain Go map (via
// attributevalue.UnmarshalMap) so standard JSON round-trips preserve N/S types.
type cursorPayload struct {
	Key map[string]any `json:"k"`
}

// decodeCursor extracts the DynamoDB ExclusiveStartKey from an opaque cursor
// query param. Returns nil (start from the beginning) on empty/invalid input.
func decodeCursor(cursor string) map[string]types.AttributeValue {
	if cursor == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil || len(payload.Key) == 0 {
		return nil
	}
	avKey, err := attributevalue.MarshalMap(payload.Key)
	if err != nil {
		return nil
	}
	return avKey
}

// buildNextCursor encodes a DynamoDB LastEvaluatedKey as the next-page cursor.
// Returns nil when key is empty (no next page).
func buildNextCursor(key map[string]types.AttributeValue) *string {
	if len(key) == 0 {
		return nil
	}
	var plainKey map[string]any
	if err := attributevalue.UnmarshalMap(key, &plainKey); err != nil {
		return nil
	}
	raw, err := json.Marshal(cursorPayload{Key: plainKey})
	if err != nil {
		return nil
	}
	return new(base64.StdEncoding.EncodeToString(raw))
}

// maxLimitParam caps every paginated endpoint's page size. Without it a
// client could ask for a page of any size — the value reaches DynamoDB's
// int32 Limit unchecked, and a huge page is expensive to read regardless.
const maxLimitParam = 100

func limitParam(c fiber.Ctx) int {
	limit := 50
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 {
		limit = min(n, maxLimitParam)
	}
	return limit
}

// sendPage writes data as a paginated JSON response. incomingCursor is the raw
// cursor query param the client sent for this request — echoed back verbatim
// as previous_cursor per the pagination convention (client resends it to page
// backwards; the server does not maintain a cursor chain).
func sendPage(c fiber.Ctx, data any, lastEvaluatedKey map[string]types.AttributeValue, incomingCursor string) error {
	var prevCursor *string
	if incomingCursor != "" {
		prevCursor = &incomingCursor
	}
	return c.JSON(PaginatedResponse{
		Data:           data,
		HasNext:        len(lastEvaluatedKey) > 0,
		NextCursor:     buildNextCursor(lastEvaluatedKey),
		HasPrevious:    incomingCursor != "",
		PreviousCursor: prevCursor,
	})
}

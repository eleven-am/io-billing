package billing

import (
	"fmt"
	"strconv"
	"strings"
)

func encodeOpValue(operationID string, amount int64) string {
	return fmt.Sprintf("%s|%d", operationID, amount)
}

func decodeOpValue(value string) (operationID string, amount int64, err error) {
	parts := strings.Split(value, "|")
	if len(parts) != 2 {
		return "", 0, ErrOperationConflict
	}
	amt, parseErr := strconv.ParseInt(parts[1], 10, 64)
	if parseErr != nil {
		return "", 0, ErrOperationConflict
	}
	return parts[0], amt, nil
}

func encodeReserveOpValue(reservationID string, amount int64) string {
	return fmt.Sprintf("%s|%d", reservationID, amount)
}

func decodeReserveOpValue(value string) (reservationID string, amount int64, err error) {
	parts := strings.Split(value, "|")
	if len(parts) != 2 {
		return "", 0, ErrOperationConflict
	}
	amt, parseErr := strconv.ParseInt(parts[1], 10, 64)
	if parseErr != nil {
		return "", 0, ErrOperationConflict
	}
	return parts[0], amt, nil
}

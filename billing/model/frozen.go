package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// FrozenDetail records which entitlement was frozen and by how much.
type FrozenDetail struct {
	EntitlementID int64 `json:"entitlement_id"`
	Seconds       int64 `json:"seconds"`
}

// FrozenDetails is a list of FrozenDetail for JSONB storage.
type FrozenDetails []FrozenDetail

func (f FrozenDetails) Value() (driver.Value, error) {
	return json.Marshal(f)
}

func (f *FrozenDetails) Scan(value interface{}) error {
	if value == nil {
		*f = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, f)
}
